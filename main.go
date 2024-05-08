package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	flags "github.com/jessevdk/go-flags"
)

var opts struct {
	Threads    int    `short:"t" long:"threads" default:"8" description:"How many threads should be used"`
	ResolverIP string `short:"r" long:"resolver" description:"IP of the DNS resolver to use for lookups"`
	Protocol   string `short:"P" long:"protocol" choice:"tcp" choice:"udp" default:"udp" description:"Protocol to use for lookups"`
	Port       uint16 `short:"p" long:"port" default:"53" description:"Port to bother the specified DNS resolver on"`
	Domain     bool   `short:"d" long:"domain" description:"Output only domains"`
}

func main() {
	_, err := flags.ParseArgs(&opts, os.Args)
	if err != nil {
		os.Exit(1)
	}

	work := make(chan string)
	wg := &sync.WaitGroup{}

	// Start worker goroutines
	for i := 0; i < opts.Threads; i++ {
		wg.Add(1)
		go doWork(work, wg)
	}

	// Read CIDR blocks from stdin and send individual IP addresses to workers
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		cidr := scanner.Text()
		ips, err := expandCIDR(cidr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing CIDR %s: %v\n", cidr, err)
			continue
		}
		for _, ip := range ips {
			work <- ip
		}
	}
	close(work)
	wg.Wait()
}

func expandCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		ips = append(ips, ip.String())
	}
	// Remove network address and broadcast address
	return ips[1 : len(ips)-1], nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func doWork(work chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	var r *net.Resolver

	if opts.ResolverIP != "" {
		r = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, opts.Protocol, fmt.Sprintf("%s:%d", opts.ResolverIP, opts.Port))
			},
		}
	}

	for ip := range work {
		addr, err := r.LookupAddr(context.Background(), ip)
		if err != nil {
			continue
		}
		for _, a := range addr {
			if opts.Domain {
				fmt.Println(strings.TrimRight(a, "."))
			} else {
				fmt.Println(ip, "\t", a)
			}
		}
	}
}
