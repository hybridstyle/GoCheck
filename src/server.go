package main

import (
	"fmt"
	"redis"
	"time"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"strconv"
	"strings"
	"net"
	"net/http"
	"net/url"
	"crypto/tls"
	"io/ioutil"
	"os"
)

const (
	redisIP string      = "10.130.209.178"
	mysqlIP string      = redisIP
	scaninternal int64  = 1 * 60 * 1000
	checkinternal int64 = 30 * 1000
	scannerqueue string = "scannerqueue"
	cleanqueue string   = "cleanqueue"
	cleanurl string     = "http://20140507.ip138.com/ic.asp"
	ipszie int          = 5
	threhold int64      = 10

	cleanthread   = 100
	cleanpoolsize = 50

	ANONY_ERROR int      = 0
	ANONY_ELITE int      = 1
	ANONY_TRANSPARNT int = 2

	SCAN_PORTS = "scanports"
)

func main() {
	fmt.Println("Server Start")


	remoteip := getremoteip()

	if remoteip == "" {
		fmt.Println("can't get remote ip")
		os.Exit(1)
	}

	db , err := sql.Open("mysql", "proxy:proxy@tcp("+mysqlIP+":3306)/proxy")
	if err != nil {
		fmt.Printf("open mysql error %s", err)
	}



	//	anony := verifyproxy("222.85.103.104:81", 20, "123")
	//	fmt.Println("anony:", anony)

	//	if false {
	cleanpool := make(chan string, cleanpoolsize)
	for i := 0; i < cleanthread; i++ {
		name := "cleaner[" + strconv.Itoa(i) + "]";
		go cleanproxy(name, cleanpool, remoteip, db)
	}
	go loadcleanpool(cleanpool)

	go loadScan(db)
	loadClean(db)
	//	}
}

func getremoteip() string {

	resp, err := http.Get(cleanurl)

	if err != nil {
		fmt.Println("getremoteip ", err)
		return ""
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	bodystr := string(body)


	start := strings.Index(bodystr, "[")
	end := strings.Index(bodystr, "]")

	if start != -1 && end != -1 {
		remoteip := bodystr[start+1:end]
		fmt.Println("remoteip:" + remoteip)
		return remoteip
	}

	return ""


}

func loadcleanpool(cleanpool chan string) {

	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("getscan error %s", err)
		}
	}()

	config := redis.DefaultSpec().Host(redisIP).Port(6379).Db(0)
	client, e := redis.NewSynchClientWithSpec(config)
	if e != nil {
		fmt.Println("failed to create redis client", e)
	}

	for {
		value, e := client.Lpop(cleanqueue)
		if e != nil {
			fmt.Println("popcleanqueue error ", e)
		}

		v := string(value)

		if v != "" {
			cleanpool <- v
		}else {
			time.Sleep(500 * time.Microsecond)
		}
	}
}

func cleanproxy(name string, cleanpool chan string, remoteip string, db *sql.DB) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("cleanproxy error %s", err)
		}
	}()

	fmt.Printf("%s start\n", name)

	delSQL := "delete from valid_proxy where ip=? and port=?"
	updateSQL := "insert into valid_proxy(ip,port,anony,create_time,update_time)values(?,?,?,?,?) on duplicate key update anony=?,update_time=?"


	for {
		select {
		case value := <-cleanpool:
			now := now()
			arr := strings.Split(value, "|")
			ip := arr[0]
			port := arr[1]
			anony := verifyproxy(value, 20, remoteip)
			count := 0
			for count < 2 {
				if anony == ANONY_ERROR {
					time.Sleep(5 * time.Second)
					anony = verifyproxy(value, 20, remoteip)
				}else {
					break;
				}
				count = count+1
			}
			fmt.Printf("%s:%s anony:%d\n", ip, port, anony)
			if anony == 0 {
				db.Exec(delSQL, ip, port)
			}else {
				db.Exec(updateSQL, ip, port, anony, now, now, anony, now)
			}
		}
	}
}

func verifyproxy(proxyinfo string, timeout int, remoteip string) int {
	proxy := proxy(proxyinfo)
	proxyUrl, err := url.Parse("http://" + proxy);
	httpClient := &http.Client {
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
			ResponseHeaderTimeout:time.Duration(timeout) * time.Second,
			DisableKeepAlives:true,
			TLSClientConfig:&tls.Config{InsecureSkipVerify:true},
			TLSHandshakeTimeout:10 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", cleanurl, nil)
	req.Header.Add("Accept", "*/*")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_9_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/37.0.2062.94 Safari/537.36")
	req.Close = true

	resp, err := httpClient.Do(req)

	if err != nil {
		fmt.Println("verifyproxy error ", err)
		return ANONY_ERROR
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	bodystr := string(body)

	titlestart := strings.Index(bodystr, "<title>")
	titleend := strings.Index(bodystr, "</title>")
	if titlestart != -1 && titleend != -1 {
		title := bodystr[titlestart+len("<title>"):titleend]

		if strings.Contains(title, "IP") {
			centerstart := strings.Index(bodystr, "<center>")
			centerend := strings.Index(bodystr, "</center")
			if centerstart != -1 && centerend != -1 {
				centercontent := bodystr[centerstart+len("<center>"):centerend]

				ipstart := strings.Index(centercontent, "[")
				ipend := strings.Index(centercontent, "]")
				if ipstart != -1 && ipend != -1 {
					ipaddr := centercontent[ipstart+1:ipend]

					if ipaddr == remoteip {
						return ANONY_TRANSPARNT
					}else {
						return ANONY_ELITE
					}
				}

			}
		}
	}

	return ANONY_ERROR

}

func proxy(proxyInfo string) string {
	tmp := strings.Replace(proxyInfo, "|", ":", -1)
	return tmp
}

func loadScan(db *sql.DB) {
	//	ports := []string{"81", "90", "808", "1080", "3128", "8000", "8080", "8123", "8888", "18186"}
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("loadScan error %s", err)
		}
	}()


	config := redis.DefaultSpec().Host(redisIP).Port(6379).Db(0)
	client, e := redis.NewSynchClientWithSpec(config)
	if e != nil {
		fmt.Println("failed to create redis client", e)
	}

	//	db , err := sql.Open("mysql", "proxy:proxy@tcp("+mysqlIP+":3306)/proxy")
	//	if err != nil {
	//		fmt.Printf("open mysql error %s", err)
	//	}

	//update segment set update_time=0
	sql := "select ip from segment where update_time < ? order by update_time"

	for {

		size, _ := client.Llen(scannerqueue)
		if size < threhold {
			scanports := getPorts(db)
			fmt.Println("scanports:", scanports)
			ports := strings.Split(scanports, "|")
			now := now() - scaninternal
			fmt.Println(now)
			rows, error := db.Query(sql, now)
			if error != nil {
				fmt.Println(error)
			}
			count := 0;
			ips := ""
			for rows.Next() {
				if count > 0 {
					ips = ips+" "
				}
				var ip string
				rows.Scan(&ip)
				ips = ips+ip
				count = count+1
				if count == ipszie {
					for _, p := range ports {
						val := p + "|" + ips;
						client.Lpush(scannerqueue, []byte(val))
					}
					count = 0
					ips = ""
				}
			}
		}else {
			time.Sleep(5 * time.Second)
		}
	}
}

func getPorts(db *sql.DB) (string) {
	sql := "select config_value from config where config_name=?"
	row := db.QueryRow(sql, SCAN_PORTS)
	var ports string
	row.Scan(&ports)
	return ports
}

func loadClean(db *sql.DB) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("loadClean error %s", err)
		}
	}()


	config := redis.DefaultSpec().Host(redisIP).Port(6379).Db(0)
	client, e := redis.NewSynchClientWithSpec(config)
	if e != nil {
		fmt.Println("failed to create redis client", e)
	}

	//	db , err := sql.Open("mysql", "proxy:proxy@tcp("+mysqlIP+":3306)/proxy")
	//	if err != nil {
	//		fmt.Printf("open mysql error %s", err)
	//	}

	sql := "select ip,port from valid_proxy where update_time < ? order by update_time"

	for {
		now := now() - checkinternal
		rows, error := db.Query(sql, now)
		if error != nil {
			fmt.Println(error)
		}

		for rows.Next() {
			var ip, port string
			rows.Scan(&ip, &port)
			val := ip + "|" + port
			client.Lpush(cleanqueue, []byte(val))

		}
		time.Sleep(1 * time.Minute)
	}
}


func now() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
