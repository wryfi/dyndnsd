# dyndnsd

This is a reimplementation of [cmur2/dyndnsd](https://github.com/cmur2/dyndnsd),
which is a ruby implementation of the DynDNS dynamic DNS update API.

## purpose

I didn't want to haul Ruby's baggage onto my DNS server, so I reimplemented
the idea in go. This implementation is entirely designed around my
personal use case, which is generating a zone for
[NSD](https://www.nlnetlabs.nl/projects/nsd/about/).

Most router firmwares support the DynDNS API implemented here, so that
when your ISP changes your dynamic external IP address, your router can
inform dyndnsd, which can update DNS records accordingly.

## installation

To install this implementation of `dyndnsd`, all you
need to do is grab the latest binary from the GitHub Releases page,
and put it on your system's path.

## configuration

The configuration is designed to be largely compatible with that of
cmur2/dyndnsd, with a couple of notable changes:

* There is (currently) only one updater, and it is called `zonefile` (you
may have used the value `command_with_bind_zone` with cmur2/dyndnsd).
* There is no `db` setting in this implementation of `dyndnsd`. In this
implementation, everything is read directly from the zone file.
* There is no `additional_zone_content` setting recognized in this
implementation. If you want additional zone content, you can edit the
zone file directly, and add it yourself. `dyndnsd` will preserve any
valid records that it finds there.
* `ttl` (and all other similar settings) must be expressed as an integer
in seconds (`5m` is not a valid setting; use `300` instead).
* `users` configuration is not parsed, as users are not currently
supported.
* `updater.params.serial_incrementer` sets the method for incrementing the
zone file serial number. Valid options are `epoch_s` to use a UNIX epoch
timestamp as the serial number, or `iso8601` to use a date-based format.
* Several additional options are added for use when dyndnsd has to create
a zone file from scratch.

### configuration file locations

dyndnsd will look for its configuration in:

* $HOME/.config/dyndnsd/config.yml
* /etc/dyndnsd/config.yml

### example configuration

```
host: 127.0.0.1
port: 8245
domain: "zyx.zig.zag"
updater:
  name: "zonefile"
  params:
    # make sure to register zone file in your nsd.conf
    zone_file: "/etc/dyndnsd/zones/zyx.zig.zag.zone"
    # fake DNS update (discards NSD stats)
    command: "nsd-control reload zyx.zig.zag"
    dns: "ns.zig.zag."
    email_addr: "hostmaster.zig.zag."
    serial_incrementer: "epoch_s"
    ttl: 300         # used for updated and newly created A and AAAA records
    refresh: 900     # only used when creating a new zone file
    retry: 300       # only used when creating a new zone file
    expire: 86400    # only used when creating a new zone file
    negttl: 900      # only used when creating a new zone file
    soattl: 1800     # only used when creating a new zone file
    nsttl: 1800      # only used when creating a new zone file
```

### serial incrementers

There are two zone serial number incrementers implemented:

1. `epoch_s` - the default, this setting always uses the current UNIX
epoch timestamp as the serial number. It does no further checking of
whether the serial number is going forward, backward, or otherwise.
1. `iso8601` - this uses a string consisting of the iso8601 date, plus
a two-digit incrementer, e.g. `2018042000` for the first serial number
on April 20, 2018. This incrementer only works up until the final two
digits hit 99, then it stops incrementing.

## modus operandi

`dyndnsd` listens on the configured port for HTTP GET requests at the
URL `/nic/update`. As part of a valid request, it expects to receive URL
parameters `hostname`, `myip`, and/or `myip6`. `hostname` is mandatory,
but `myip` and `myip6` are optional. A complete request using curl might
look like:

```
curl 'http://localhost:8245/nic/update?hostname=ziggy.zyx.zig.zag&myip=192.123.230.41&myip6=2003:0db8:85a3:0000:0000:8a2e:0370:7344'
```

The above request will cause dyndnsd to read its configured zone file.
If there is an existing A record matching `ziggy.zyx.zig.zag`, it will
be updated to point to `192.123.239.41`. Likewise, if there is an existing
AAAA record, it will be updated. If either record does not exist, it will
be added accordingly.

If both `myip` and `myip6` are omitted from the request, `dyndnsd` will
register the IP address from the `X-Forwarded-For` header (if set) or
from the request IP (if not).

The zone file will be updated accordingly, and its serial number will be
incremented.

Finally, the command specified in the configuration section
`updater.params.command` will be run.


### data preservation

As much as possible `dyndnsd` will preserve whatever data it finds in its
zone file. It will only update the SOA record serial number, and any
matching A or AAAA records. All other valid records are preserved as-is.
It should be safe, therefore, to add whatever custom records you want
directly to the zone file. As long as they are not A or AAAA records
that match incoming requests, they should pass through unaffected.

`dyndnsd` does not routinely update SOA fields except for the serial
number. The SOA-related settings in the configuration file are for
occasions when no zone file exists, and `dyndnsd` must generate a
valid zone file from scratch.

## missing features

Compared to cmur2/dyndnsd, this implementation is missing at least the
following features:

* Users and access controls

## deployment

It is recommended to run `dyndnsd` so that it listens only on `localhost`
(the default is `localhost:8245`), and then use a reverse proxy like
nginx or caddy to add security, like https and basic auth, in front of it.