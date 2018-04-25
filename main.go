package main

import (
	"bufio"
	"path/filepath"
	"fmt"
	"net/http"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/mitchellh/go-homedir"
	"github.com/codeskyblue/go-sh"
	"github.com/spf13/viper"
)


func main() {
	home, err := homedir.Dir()
	if err != nil {
		log.Fatal(err)
	}
	viper.SetConfigName("config")
	viper.AddConfigPath(filepath.Join(home, ".config", "dyndnsd"))
	viper.AddConfigPath("/etc/dyndnsd")
	viper.SetDefault("host", "127.0.0.1")
	viper.SetDefault("port", "8245")
	viper.SetDefault("domain", "zyx.zig.zag")
	viper.SetDefault("updater.name", "zonefile")
	viper.SetDefault("updater.params.zone_file", "zyx.zig.zag.zone")
	viper.SetDefault("updater.params.dns", "ns.zig.zag.")
	viper.SetDefault("updater.params.email_addr", "hostmaster.zig.zag.")
	viper.SetDefault("updater.params.serial_incrementer", "epoch_s")
	viper.SetDefault("updater.params.ttl", 300)
	viper.SetDefault("updater.params.refresh", 900)
	viper.SetDefault("updater.params.retry", 300)
	viper.SetDefault("updater.params.expire", 86400)
	viper.SetDefault("updater.params.negttl", 900)
	viper.SetDefault("updater.params.soattl", 1800)
	viper.SetDefault("updater.params.nsttl", 1800)
	err = viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Fatal error with config file: %s \n", err)
	}
	listen := fmt.Sprintf("%s:%s", viper.GetString("host"), viper.GetString("port"))
	log.Printf("listening on %s \n", listen)
	http.HandleFunc("/nic/update", updateHandler)
	log.Fatal(http.ListenAndServe(listen, nil))
}


func updateHandler(response http.ResponseWriter, request *http.Request) {
	requestParams := processUrlParams(response, request)
	if len(requestParams) >= 1 {
		if _, ok := requestParams["ip4"]; !ok {
			if _, ok := requestParams["ip6"]; !ok {
				log.Println("no valid ip address found in URL parameters, falling back on request metadata")
				var address net.IP
				xff := request.Header.Get("X-Forwarded-For")
				if len(xff) > 0 {
					addresses := strings.Split(xff, ",")
					address = net.ParseIP(addresses[0])
				} else {
					address = net.ParseIP(strings.Split(request.RemoteAddr, ":")[0])
				}
				if address.To4() != nil {
					requestParams["ip4"] = address.String()
				} else if address.To16() != nil {
					requestParams["ip6"] = address.String()
				}
			}
		}
		if viper.GetString("updater.name") == "zonefile" {
			log.Println("updating zonefile")
			zonefileUpdater(requestParams)
		} else {
			log.Println("no supported updater was set in config")
		}
	} else {
		log.Println("empty parameters")
	}
}


func processUrlParams(response http.ResponseWriter, request *http.Request) map[string]string {
	domain := viper.GetString("domain")
	params := make(map[string]string)
	hostnames, hostnameOk := request.URL.Query()["hostname"]
	if hostnameOk && len(hostnames) == 1 {
		if strings.HasSuffix(hostnames[0], domain) && hostnames[0] != domain {
			if !strings.HasSuffix(hostnames[0], ".") {
				hostnames[0] = fmt.Sprintf("%s.", hostnames[0])
			}
			params["hostname"] = hostnames[0]
		}
	}
	if _, ok := params["hostname"]; !ok {
		response.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(response, "{\"error\": \"missing or invalid hostname\"}")
		return map[string]string{}
	}
	ip4addrs, ip4addrsOk := request.URL.Query()["myip"]
	if ip4addrsOk && len(ip4addrs) == 1 {
		if net.ParseIP(ip4addrs[0]) != nil {
			params["ip4"] = ip4addrs[0]
		}
	}
	ip6addrs, ip6addrsOk := request.URL.Query()["myip6"]
	if ip6addrsOk && len(ip6addrs) == 1 {
		if net.ParseIP(ip6addrs[0]) != nil {
			params["ip6"] = ip6addrs[0]
		}
	}
	return params
}


func zonefileUpdater(params map[string]string) {
	zonefile := viper.GetString("updater.params.zone_file")
	domain := viper.GetString("domain")
	if _, err := os.Stat(zonefile); err == nil {
		log.Printf("found zone file: %s", zonefile)
		updateExistingZone(zonefile, domain, params)
	} else {
		log.Printf("creating new zone file: %s", zonefile)
		createNewZone(zonefile, domain, params)
	}
	command := viper.GetString("updater.params.command")
	if len(command) > 0 {
		fields := strings.Fields(command)
		out, err := sh.Command(fields[0], fields[1:]).Output()
		if err != nil {
			log.Println(err)
		} else {
			fmt.Println(string(out))
		}
	}
}


func updateExistingZone(zonefile string, domain string, params map[string]string) {
	var records []dns.RR
	var newRecords []dns.RR
	foundA := false
	found6A := false
	zoneopener, err := os.Open(zonefile)
	if err == nil {
		log.Println("parsing zone file")
		for token := range dns.ParseZone(zoneopener, domain, zonefile) {
			if token.Error != nil {
				log.Println(token.Error)
			} else {
				records = append(records, token.RR)
			}
		}
	}
	zoneopener.Close()
	for _, record := range records {
		header := record.Header()
		if header.Rrtype == dns.TypeSOA {
			typeRecord := record.(*dns.SOA)
			typeRecord.Serial = incrementSerial(typeRecord.Serial)
			newRecords = append(newRecords, record)
		} else if header.Rrtype == dns.TypeA && header.Name == params["hostname"] {
			foundA = true
			if _, ok := params["ip4"]; ok {
				typeRecord := record.(*dns.A)
				typeRecord.A = net.ParseIP(params["ip4"])
			}
			newRecords = append(newRecords, record)
		} else if header.Rrtype == dns.TypeAAAA && header.Name == params["hostname"] {
			found6A = true
			if _, ok := params["ip6"]; ok {
				typeRecord := record.(*dns.AAAA)
				typeRecord.AAAA = net.ParseIP(params["ip6"])
			}
			newRecords = append(newRecords, record)
		} else {
			newRecords = append(newRecords, record)
		}
	}
	if !foundA {
		if _, ok := params["ip4"]; ok {
			log.Println("adding new A record")
			ttl := viper.GetInt("updater.params.ttl")
			record := new(dns.A)
			record.Hdr = dns.RR_Header{Name: params["hostname"], Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(ttl)}
			record.A = net.ParseIP(params["ip4"])
			newRecords = append(newRecords, record)
		}
	}
	if !found6A {
		if _, ok := params["ip6"]; ok {
			log.Println("adding new AAAA record")
			ttl := viper.GetInt("updater.params.ttl")
			record := new(dns.AAAA)
			record.Hdr = dns.RR_Header{Name: params["hostname"], Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: uint32(ttl)}
			record.AAAA = net.ParseIP(params["ip6"])
			newRecords = append(newRecords, record)
		}
	}
	zonewriter, _ := os.Create(zonefile)
	defer zonewriter.Close()
	writer := bufio.NewWriter(zonewriter)
	for _, record := range newRecords {
		fmt.Fprintln(writer, record)
	}
	writer.Flush()
}


func incrementSerial(serial uint32) uint32 {
	incrementer := viper.GetString("updater.params.serial_incrementer")
	if incrementer == "epoch_s" {
		serial := time.Now().Unix()
		return uint32(serial)
	} else if incrementer == "iso8601" {
		// serial is a uint32; convert to string
		serialStr := strconv.Itoa(int(serial))
		today := time.Now().UTC().Format("20060102")
		// if the existing serial starts with today's date, increment it by 1;
		// otherwise append 00 to today's date, and set that as the new serial
		if strings.HasPrefix(serialStr, today) {
			if !strings.HasSuffix(serialStr, "99") {
				return serial + 1
			} else {
				log.Println("cannot increment iso8601 serial past 99")
				// TODO: return an err
				return serial
			}
		} else {
			newSerial, _ := strconv.Atoi(fmt.Sprintf("%s00", today))
			return uint32(newSerial)
		}
	}
	return serial
}

func createNewZone(zonefile string, domain string, params map[string]string) {
	var records []dns.RR
	soattl := viper.GetInt("updater.params.soattl")
	soa := new(dns.SOA)
	soa.Hdr = dns.RR_Header{Name: fmt.Sprintf("%s.", domain), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: uint32(soattl)}
	soa.Refresh = uint32(viper.GetInt("updater.params.refresh"))
	soa.Retry = uint32(viper.GetInt("updater.params.retry"))
	soa.Expire = uint32(viper.GetInt("updater.params.expire"))
	soa.Minttl = uint32(viper.GetInt("updater.params.negttl"))
	soa.Mbox = viper.GetString("updater.params.email_addr")
	soa.Ns = viper.GetString("updater.params.dns")
	soa.Serial = incrementSerial(0)
	records = append(records, soa)

	nsttl := viper.GetInt("updater.params.nsttl")
	ns := new(dns.NS)
	ns.Hdr = dns.RR_Header{Name: fmt.Sprintf("%s.", domain), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: uint32(nsttl)}
	ns.Ns = viper.GetString("updater.params.dns")
	records = append(records, ns)

	ttl := viper.GetInt("updater.params.ttl")
	if _, ok := params["ip4"]; ok {
		record := new(dns.A)
		record.Hdr = dns.RR_Header{Name: params["hostname"], Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(ttl)}
		record.A = net.ParseIP(params["ip4"])
		records = append(records, record)
	}

	if _, ok := params["ip6"]; ok {
		record := new(dns.AAAA)
		record.Hdr = dns.RR_Header{Name: params["hostname"], Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: uint32(ttl)}
		record.AAAA = net.ParseIP(params["ip6"])
		records = append(records, record)
	}

	zoneopener, _ := os.Create(zonefile)
	defer zoneopener.Close()
	writer := bufio.NewWriter(zoneopener)
	for _, record := range records {
		fmt.Fprintln(writer, record)
	}
	writer.Flush()
}