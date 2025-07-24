package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"
)

const (
	PORT              = ":7001"
	BATCH_FLUSH_COUNT = 10
	BATCH_FLUSH_TIME  = 10 * time.Second

	influxURL = "http://influxdb2:8086"
	token     = "2bq9r-S3qb3e-ter4-w2kid"
	org       = "beacon"
	bucketBcn = "beacon"

	prefix    = "00:C0:B1"
)

type BeaconData struct {
	HygateMAC string
	BeaconMAC string
	RSSI      int
	Timestamp time.Time
	RemoteIP  string
}

var (
	influxClient      influxdb2.Client
	influxWriteBeacon influxdb2api.WriteAPIBlocking
	influxQueue       = make(chan BeaconData, 500)
)

func main() {
	initInflux()
	go influxWorker()

	ln, err := net.Listen("tcp", PORT)
	if err != nil {
		panic(err)
	}
	fmt.Println("✅ TCP Server listening on", PORT)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	clientIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()
	fmt.Printf("[CONNECTED] %s\n", clientIP)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()

		fmt.Printf("[RAWDATA] %s\n", line)

		if strings.HasPrefix(line, "PROXY") {
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				clientIP = parts[2]
			}
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 5 {
			continue
		}

		bmac, err1 := formatMAC(parts[1])
		hmac, err2 := formatMAC(parts[4])
		rssi := parseRSSI(parts[2])
		if err1 != nil || err2 != nil {
			continue
		}

		if !strings.HasPrefix(bmac, prefix) {
			continue // prefix filter
		}

		data := BeaconData{hmac, bmac, rssi, time.Now(), clientIP}
		influxQueue <- data
	}
}

func influxWorker() {
	batch := []BeaconData{}
	ticker := time.NewTicker(BATCH_FLUSH_TIME)
	for {
		select {
		case data := <-influxQueue:
			batch = append(batch, data)
			if len(batch) >= BATCH_FLUSH_COUNT {
				flushBeacon(batch)
				batch = []BeaconData{}
			}
		case <-ticker.C:
			if len(batch) > 0 {
				flushBeacon(batch)
				batch = []BeaconData{}
			}
		}
	}
}

func flushBeacon(batch []BeaconData) {
	for _, d := range batch {
		p := influxdb2.NewPointWithMeasurement("mem").
			AddTag("hygate_mac", d.HygateMAC).
			AddTag("ip", d.RemoteIP).
			AddTag("mac", d.BeaconMAC).
			AddField("rssi", d.RSSI).
			SetTime(d.Timestamp)

		err := influxWriteBeacon.WritePoint(context.Background(), p)
		if err != nil {
			fmt.Println("❌ Influx write error:", err)
		}
	}
}

func formatMAC(raw string) (string, error) {
	if strings.HasPrefix(raw, "0x") {
		raw = raw[2:]
	}
	if len(raw) != 12 {
		return "", fmt.Errorf("invalid MAC length")
	}
	parts := []string{}
	for i := 0; i < 12; i += 2 {
		parts = append(parts, raw[i:i+2])
	}
	return strings.ToUpper(strings.Join(parts, ":")), nil
}

func parseRSSI(r string) int {
	var val int
	fmt.Sscanf(r, "%d", &val)
	return val
}

func initInflux() {
	influxClient = influxdb2.NewClient(influxURL, token)
	influxWriteBeacon = influxClient.WriteAPIBlocking(org, bucketBcn)
	fmt.Println("✅ InfluxDB Connected")
}