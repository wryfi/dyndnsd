package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"strings"
	"net"
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
					address = net.ParseIP(xff)
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
		log.Println(requestParams)
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


