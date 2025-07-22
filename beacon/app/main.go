package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BeaconData struct {
	HygateMAC string
	BeaconMAC string
	RSSI      int
	Timestamp int64
	RemoteIP  string
}

type OwnerInfo struct {
	HygateMAC string
	RSSI      int
	Timestamp int64
}

var (
	VALID_HYGATE_MACS = map[string]bool{}
	VALID_BEACON_MACS = map[string]bool{}
	UNREGISTERED_MACS = map[string]bool{}

	beaconOwnerMap = make(map[string]OwnerInfo)
	ownerLock      = sync.Mutex{}

	// DB clients
	mongoClient    *mongo.Client
	influxClient   influxdb2.Client
	influxWriteAPI influxdb2api.WriteAPIBlocking
)

func main() {
	initMongo()
	initInflux()
	initValidMACsFromMongo()
	
	listener, err := net.Listen("tcp", ":7001")
	if err != nil {
		panic(err)
	}
	fmt.Println("[TCP SERVER] Listening on port 7001")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("[ERROR] Accept:", err)
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
		raw := scanner.Text()
		parts := strings.Split(raw, ",")
		if len(parts) != 5 {
			continue
		}
		beaconRaw := parts[1]
		rssi := parts[2]
		hygateRaw := parts[4]

		beaconMAC, err1 := formatMAC(beaconRaw)
		hygateMAC, err2 := formatMAC(hygateRaw)
		if err1 != nil || err2 != nil {
			continue
		}

		if !isValidHygate(hygateMAC, clientIP) || !isValidBeacon(beaconMAC) {
			continue
		}

		rssiVal := parseRSSI(rssi)
		data := BeaconData{
			HygateMAC: hygateMAC,
			BeaconMAC: beaconMAC,
			RSSI:      rssiVal,
			Timestamp: time.Now().UnixNano(),
			RemoteIP:  clientIP,
		}

		if isOwner(data) {
			fmt.Printf("[SAVE] %s -> %s (%d)\n", hygateMAC, beaconMAC, rssiVal)
			saveToInflux(data)
		}
	}

	fmt.Printf("[DISCONNECTED] %s\n", clientIP)
}

func formatMAC(raw string) (string, error) {
	if strings.HasPrefix(raw, "0x") {
		raw = raw[2:]
	}
	if len(raw) != 12 {
		return "", fmt.Errorf("invalid MAC length")
	}
	var formatted strings.Builder
	for i := 0; i < 12; i += 2 {
		formatted.WriteString(raw[i : i+2])
		if i < 10 {
			formatted.WriteString(":")
		}
	}
	return formatted.String(), nil
}

func parseRSSI(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func isValidHygate(mac string, ip string) bool {
	if VALID_HYGATE_MACS[mac] {
		return true
	}
	if UNREGISTERED_MACS[mac] {
		return false
	}

	fmt.Printf("[UNREGISTERED HYGATE] %s from IP %s\n", mac, ip)
	UNREGISTERED_MACS[mac] = true
	return false
}

func isValidBeacon(mac string) bool {
	if VALID_BEACON_MACS[mac] {
		return true
	}
	if UNREGISTERED_MACS[mac] {
		return false
	}
	UNREGISTERED_MACS[mac] = true
	return false
}

func isOwner(data BeaconData) bool {
	now := time.Now().Unix()
	ownerLock.Lock()
	defer ownerLock.Unlock()

	info, exists := beaconOwnerMap[data.BeaconMAC]

	if !exists || now-info.Timestamp > 10 {
		beaconOwnerMap[data.BeaconMAC] = OwnerInfo{data.HygateMAC, data.RSSI, now}
		return true
	}

	if data.RSSI > info.RSSI || info.HygateMAC == data.HygateMAC {
		beaconOwnerMap[data.BeaconMAC] = OwnerInfo{data.HygateMAC, data.RSSI, now}
		return true
	}
	return false
}

func saveToInflux(data BeaconData) {
	point := influxdb2.NewPointWithMeasurement("mem").
		AddTag("hygate_mac", data.HygateMAC).
		AddTag("ip", data.RemoteIP).
		AddTag("mac", data.BeaconMAC).
		AddField("rssi", data.RSSI).
		SetTime(time.Unix(0, data.Timestamp))

	err := influxWriteAPI.WritePoint(context.Background(), point)
	if err != nil {
		fmt.Printf("[INFLUX ERROR] %v\n", err)
	}
}

func initMongo() {
	var err error
	uri := "mongodb://smhanduck:hd123@test-mongodb:27017"

	mongoClient, err = mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Println("❌ MongoDB Connection Failed (connect):", err)
		os.Exit(1)
	}

	err = mongoClient.Ping(context.Background(), nil)
	if err != nil {
		fmt.Println("❌ MongoDB Connection Failed (ping):", err)
		os.Exit(1)
	}

	fmt.Println("✅ MongoDB Connected")
}

func initInflux() {
	url := "http://test-influxdb2:8086"
	token := "2bq9r-S3qb3e-ter4-w2kid"
	org := "smhanduck"

	influxClient = influxdb2.NewClient(url, token)
	influxWriteAPI = influxClient.WriteAPIBlocking(org, "smhanduck_beacon")

	health, err := influxClient.Health(context.Background())
	if err != nil || health.Status != "pass" {
		fmt.Println("❌ InfluxDB Connection Failed:", err)
		os.Exit(1)
	}
	fmt.Println("✅ InfluxDB Connected")
}

func initValidMACsFromMongo() {
	ctx := context.Background()
	db := mongoClient.Database("SMHANDUCK")

	collections := []string{"vehicle", "worker", "heartbit", "scanner"}
	for _, col := range collections {
		cursor, err := db.Collection(col).Find(ctx, map[string]any{})
		if err != nil {
			fmt.Printf("⚠️ Mongo Find Error: %s\n", col)
			continue
		}
		for cursor.Next(ctx) {
			var doc struct {
				MAC string `bson:"mac"`
			}
			if err := cursor.Decode(&doc); err == nil && doc.MAC != "" {
				if col == "scanner" {
					VALID_HYGATE_MACS[doc.MAC] = true
				} else {
					VALID_BEACON_MACS[doc.MAC] = true
				}
			}
		}
		cursor.Close(ctx)
	}

	// Load unregistered MACs
	cursor, err := db.Collection("unregisteredDevice").Find(ctx, map[string]any{})
	if err != nil {
		fmt.Println("⚠️ Mongo Find Error: unregisteredDevice")
		return
	}
	for cursor.Next(ctx) {
		var doc struct {
			MAC string `bson:"mac"`
		}
		if err := cursor.Decode(&doc); err == nil && doc.MAC != "" {
			UNREGISTERED_MACS[doc.MAC] = true
		}
	}
	cursor.Close(ctx)

	fmt.Printf("Loaded %d HYGATE, %d BEACON, %d UNREGISTERED MACs\n", len(VALID_HYGATE_MACS), len(VALID_BEACON_MACS), len(UNREGISTERED_MACS))
}