package main

import (
	"fmt"
	"redis"
	"time"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"

)

const (
	redisIP string      = "10.130.210.245"
	mysqlIP string      = redisIP
	scaninternal int64  = 1 * 60 * 1000
	checkinternal int64 = 30 * 1000
	scannerqueue string = "scannerqueue"
	cleanqueue string   = "cleanqueue"

	ipszie int     = 5
	threhold int64 = 10

)

func main() {
	fmt.Println("Server Start")
	go loadScan()
	loadClean()
}


func loadScan() {
	ports := []string{"81", "90", "808", "1080", "3128", "8000", "8080", "8123", "8888", "18186"}
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

	db , err := sql.Open("mysql", "proxy:proxy@tcp("+mysqlIP+":3306)/proxy")
	if err != nil {
		fmt.Printf("open mysql error %s", err)
	}

	//update segment set update_time=0
	sql := "select ip from segment where update_time < ? order by update_time"

	for {

		size, _ := client.Llen(scannerqueue)
		if size < threhold {
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
						//						fmt.Println(val)
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

func now() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func loadClean() {
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

	db , err := sql.Open("mysql", "proxy:proxy@tcp("+mysqlIP+":3306)/proxy")
	if err != nil {
		fmt.Printf("open mysql error %s", err)
	}

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
