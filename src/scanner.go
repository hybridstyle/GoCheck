package main

import (
	"fmt"
	"redis"
	"time"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"strings"
	"os/exec"
	"os"
	"bufio"
	"strconv"
)

const (
	redisIP  string   = "10.130.209.178"
	mysqlIP string    = redisIP
	scansize int      = 100
	resultsize int    = 10
	threadsize int    = 5
	path string       = "/macken/proxycheck/results"
	checkqueue string = "checkqueue"
	scanqueue string  = "scannerqueue"
)

func main() {
	fmt.Println("Scanner Start")
	scanpool := make(chan string, scansize)
	resultpool := make(chan string, resultsize)



	for i := 0; i < threadsize; i++ {
		name := "scanner[" + strconv.Itoa(i) + "]"
		go scan(name, scanpool, resultpool)
	}

	go writeresult(resultpool)

	getscan(scanpool)

}

func scan(name string, scanpool chan string, resultpool chan string) {
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

	file := path + "/" + name + ".csv"

	for {
		select {
		case value := <-scanpool:
			//zmap -o file -p port ip1 ip2 ip3 ip4 ip5
			//port|ip1|ip2|ip3
			arr := strings.Split(value, "|")

			port := arr[0]
			ips := arr[1]

			//			cmd := "zmap -o " + file + " -f \"saddr,sport\" -p " + port + " " + ips
			cmd := "sh /macken/proxycheck/newzmapscan.sh " + file + " " + port + " " + strings.Replace(ips, " ", ",", -1)
			iparr := strings.Split(cmd, " ")
			parm := iparr[1:]
			head := iparr[0]
			fmt.Println(cmd)
			err := exec.Command(head, parm...).Run()
			if err != nil {
				fmt.Printf("zmap error %s %s\n", cmd, err)
				time.Sleep(1 * time.Minute)
				continue
			}

			f, err := os.Open(file)
			if err != nil {
				fmt.Println("open file error %s %s\n", file, err)
				time.Sleep(1 * time.Minute)
			}
			scanner := bufio.NewScanner(f)
			index := 0
			m := make(map[string]int)
			for scanner.Scan() {
				if index > 0 {
					scannerText := scanner.Text()
					key := getkey(scannerText)
					val, ok := m[key]
					if ok {
						m[key] = val+1
					}else {
						m[key] = 0
					}
					text := strings.Replace(scannerText, ",", "|", -1)
					client.Lpush(checkqueue, []byte(text))
				}
				index = index+1
			}

		for k, v := range m {
			val := k + "|" + strconv.Itoa(v)
			resultpool<-val
		}
			f.Close()

			//			val := value + "|" + strconv.Itoa(index)
			//		resultpool<-val
			//			f.Close()

		}
	}

}

func getkey(text string) (string) {
	arr := strings.Split(text, ".")
	return arr[0] + "." + arr[1] + ".0.0/16"
}

func getscan(pool chan string) {
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
		value, e := client.Lpop(scanqueue)
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

func now() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func writeresult(writepool chan string) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("writeclean error %s", err)
		}
	}()
	db , err := sql.Open("mysql", "proxy:proxy@tcp("+mysqlIP+":3306)/proxy")
	if err != nil {
		fmt.Printf("open mysql error %s", err)
	}

	updatesql := "update segment set update_time=? where ip=?";
	scansql := "insert into scan_log(ip,port,num,create_time) values(?,?,?,?)"


	for {
		select {
		case value := <-writepool:
			vals := strings.Split(value, "|")
			nowtime := now()
			_, err := db.Exec(scansql, vals[1], vals[0], vals[2], nowtime)
			if err != nil {
				fmt.Println("scansql error %s", err)
			}
			ips := vals[1]
			ipsarr := strings.Split(ips, " ")
		for _, s := range ipsarr {
			_, err := db.Exec(updatesql, nowtime, s)
			if err != nil {
				fmt.Println("updatesql error %s", err)
			}
		}

		}
	}
}
