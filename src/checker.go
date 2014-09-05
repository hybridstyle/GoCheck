package main

import (
	"fmt"
	//"log"
	"flag"
	"os"
	"redis"
	"strconv"
	"strings"
	"time"
	"runtime/pprof"
	"runtime"
	"net"
	"net/http"
	"net/url"
	"crypto/tls"
//	"io/ioutil"
)

const (
	checkUrl string    = "http://www.baidu.com"
	threadsize     int = 5000
	redisIP  string    = "10.130.209.178"
	bws string         = "BWS"
	checksize int      = 2 * threadsize
	cleansize int      = 50
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var memprofile = flag.String("memprofile", "", "memoroy profile to file")
var memprofilerate = flag.Int("memprofilerate", 0, "memprofilerate")

func main() {
	flag.Parse()


	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can not create cpu profile output file: %s",
				err)
			return
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "Can not start cpu profile: %s", err)
			f.Close()
			return
		}
		defer pprof.StopCPUProfile()
	}

	if *memprofile != "" && *memprofilerate > 0 {
		runtime.MemProfileRate = *memprofilerate
	}

	defer func() {
		if *memprofile != "" {
			f, err := os.Create(*memprofile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Can not create mem profile output file: %s", err)
				return
			}
			if err = pprof.WriteHeapProfile(f); err != nil {
				fmt.Fprintf(os.Stderr, "Can not write %s: %s", *memprofile, err)
			}
			f.Close()
		}
	}()



	chs := make(chan int)
	checkpool := make(chan string, checksize)
	cleanpool := make(chan string, cleansize)

	go getproxy(checkpool)
	go writeclean(cleanpool)

	for i := 0; i < threadsize; i++ {
		name := "checker[" + strconv.Itoa(i) + "]"
		go check(name, chs, checkpool, cleanpool)
	}
	count := 1
	for {
		select {
		case <-chs:
			count = count+1
			if count == threadsize {
				close(chs)
				return
			}
		}
	}
}

func check(name string, ch chan int, checkpool chan string, cleanpool chan string) {

	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("%s run error %s", name, err)
		}
	}()


	fmt.Printf("%s start\n", name)

	for {
		select {
		case value := <-checkpool:
			if value == "" {
				continue
			}
			proxyinfo := proxy(value)
			res := checkProxy(proxyinfo, 20)
			if res {
				cleanpool <- value
				fmt.Printf("%s %s success\n", name, value)
			}
		}
	}


	fmt.Printf("%s end\n", name)
	ch <- 1
}

func checkProxy(proxy string, timeout int) bool {
	proxyUrl, err := url.Parse("http://" + proxy)
	httpClient := &http.Client{
		Transport:&http.Transport{
			Proxy:http.ProxyURL(proxyUrl),
			Dial:func(netw, addr string) (net.Conn, error) {
				deadline := time.Now().Add(time.Duration(timeout) * time.Second)
				c, err := net.DialTimeout(netw, addr, time.Second*time.Duration(timeout))
				if err != nil {
					return nil, err
				}
				c.SetDeadline(deadline)
				return c, nil
			},
			ResponseHeaderTimeout: time.Duration(timeout) * time.Second,
			DisableKeepAlives: true,
			//			DisableCompression: true,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	req, err := http.NewRequest("HEAD", checkUrl, nil)
	req.Header.Add("Accept", "*/*")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_9_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/37.0.2062.94 Safari/537.36")

	req.Close = true

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("checkproxy error %s\n", err)
		return false
	}
	defer resp.Body.Close()

//	ioutil.ReadAll(resp.Body)

	headers := resp.Header["Server"]
	if headers != nil {
		serverHeader := headers[0]
		if strings.Contains(serverHeader, bws) {
			return true
		}
	}

	return false
}


func getproxy(pool chan string) {

	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("getproxy error %s", err)
		}
	}()

	config := redis.DefaultSpec().Host(redisIP).Port(6379).Db(0)
	client, e := redis.NewSynchClientWithSpec(config)
	if e != nil {
		fmt.Println("failed to create redis client", e)
	}

	for {
		value, e := client.Lpop("checkqueue")
		if e != nil {
			fmt.Println("get redis value error", e)
			continue
		}

		v := string(value)
		if v != "" {
			pool <- string(v)
		}else {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func writeclean(cleanpool chan string) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("writeclean error %s", err)
		}
	}()
	config := redis.DefaultSpec().Host(redisIP).Port(6379).Db(0)
	client, e := redis.NewSynchClientWithSpec(config)
	if e != nil {
		fmt.Println("failed to create redis client", e)
	}
	for {
		select {
		case value := <-cleanpool:
			client.Lpush("cleanqueue", []byte(value))
		}
	}
}

func proxy(proxyInfo string) string {
	tmp := strings.Replace(proxyInfo, "|", ":", -1)
	return tmp
}
