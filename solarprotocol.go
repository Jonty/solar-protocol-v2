// go build -ldflags "-extldflags '-lm -lstdc++ -static'" .
package main

// TODO
// * Periodically query for all nameservers for a domain
// * Have a HTTP API that serves the current value
// * Have a HTTP server that serves all the static content

import (
	"encoding/json"
	"fmt"
	"github.com/miekg/dns"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type Host struct {
	Mac       string `json:"mac"`
	Ip        string `json:"ip"`
	Timestamp string `json:"time stamp"`
	Name      string `json:"name"`
	Voltage   float64
}

var hosts []Host

func main() {
	go getLiveHosts()

	dns.HandleFunc(".", handleDNSRequest)

	go func() {
		srv := &dns.Server{Addr: ":53", Net: "udp"}
		err := srv.ListenAndServe()
		if err != nil {
			log.Fatal("Failed to set udp listener %s\n", err.Error())
		}
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case s := <-sig:
			log.Fatalf("Signal (%d) received, stopping\n", s)
		}
	}
}

func getLiveHosts() {
	for {
		jsonFile, err := os.Open("deviceList.json")
		if err != nil {
			fmt.Println(err)
		}
		byteValue, _ := ioutil.ReadAll(jsonFile)
		jsonFile.Close()

		json.Unmarshal(byteValue, &hosts)
		for i := 0; i < len(hosts); i++ {
			resp, err := http.Get("http://" + hosts[i].Ip + "/api/v1/api.php?value=PV-voltage")
			if err != nil {
				log.Println("Failed to contact " + hosts[i].Name + " on " + hosts[i].Ip)
				continue
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Fatalln(err)
			}

			voltage, err := strconv.ParseFloat(string(body), 64)
			hosts[i].Voltage = voltage

			fmt.Println("Live host: ", hosts[i])
		}

		time.Sleep(30 * time.Second)
	}
}

func getHighestVoltageHost() Host {
	var highest Host
	for i := 0; i < len(hosts); i++ {
		if highest.Voltage < hosts[i].Voltage {
			highest = hosts[i]
		}
	}

	return highest
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	domain := r.Question[0].Name

	t := time.Now()
	ip, _, _ := net.SplitHostPort(w.RemoteAddr().String())
	fmt.Printf("%d-%02d-%02d_%02d:%02d:%02d\t%s\t%s\n", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), ip, domain)

	m := new(dns.Msg)
	m.SetReply(r)
	if domain == "solarprotocol.net." {
		m.Authoritative = true
		rr1 := new(dns.A)
		rr1.Hdr = dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(60)}
		rr1.A = net.ParseIP(getHighestVoltageHost().Ip)
		m.Answer = []dns.RR{rr1}
	}
	w.WriteMsg(m)
}
