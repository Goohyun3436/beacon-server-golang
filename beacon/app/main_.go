package main

import (
	"bufio"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

type ClientInfo struct {
	LastSeen   time.Time
	LastBeacon string
}

var (
	clients = make(map[string]*ClientInfo)
	mutex   = sync.RWMutex{}
)

func main() {
	listen, err := net.Listen("tcp", "0.0.0.0:7001")
	if err != nil {
		panic(err)
	}
	fmt.Println("🚀 TCP Server listening on 7001")

	go displayClientTable()

	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println("Accept error:", err)
			continue
		}
		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	ip := conn.RemoteAddr().(*net.TCPAddr).IP.String()

	fmt.Printf("[CONNECTED] %s\n", ip)
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ",")

		if len(fields) < 5 {
			continue
		}

		beaconMAC := fields[1]
		hygateMAC := fields[4]

		if !strings.HasPrefix(beaconMAC, "0x00C0B1") || !strings.HasPrefix(hygateMAC, "0x00C0B1") {
			continue
		}

		mutex.Lock()
		clients[ip] = &ClientInfo{
			LastSeen:   time.Now(),
			LastBeacon: beaconMAC,
		}
		mutex.Unlock()
	}

	fmt.Printf("[DISCONNECTED] %s\n", ip)
	mutex.Lock()
	delete(clients, ip)
	mutex.Unlock()
}

func displayClientTable() {
	for {
		time.Sleep(1 * time.Second)

		mutex.RLock()
		fmt.Printf("\n📡 연결된 클라이언트 - %s\n", time.Now().Format("15:04:05"))
		fmt.Println("----------------------------------------------------------")
		fmt.Printf("%-4s | %-15s | %-12s | %s\n", "No.", "IP", "Last Seen", "Last Beacon")
		fmt.Println("----------------------------------------------------------")

		ips := make([]string, 0, len(clients))
		for ip := range clients {
			ips = append(ips, ip)
		}
		sort.Slice(ips, func(i, j int) bool {
			return ipLess(ips[i], ips[j])
		})

		i := 1
		now := time.Now()
		for _, ip := range ips {
			info := clients[ip]
			elapsed := now.Sub(info.LastSeen).Round(time.Second)
			fmt.Printf("%-4d | %-15s | %-12s | %s\n", i, ip, fmt.Sprintf("%s 전", elapsed), info.LastBeacon)
			i++
		}
		if i == 1 {
			fmt.Println("No clients connected.")
		}
		fmt.Println("----------------------------------------------------------")
		mutex.RUnlock()
	}
}

func ipLess(a, b string) bool {
	ipA := net.ParseIP(a).To4()
	ipB := net.ParseIP(b).To4()
	if ipA == nil || ipB == nil {
		return a < b
	}
	for i := 0; i < 4; i++ {
		if ipA[i] != ipB[i] {
			return ipA[i] < ipB[i]
		}
	}
	return false
}