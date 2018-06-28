package main

import (
	"log"
	"net"
	"strings"
)

func main() {
	var addr *net.UDPAddr
	var err error
	if addr, err = net.ResolveUDPAddr("udp4", "239.255.255.250:1900"); err != nil {
		panic(err)
	}

	var myIface *net.Interface

	//interfaces, err := net.Interfaces()
	//if err != nil {
	//	panic(err)
	//}
	//
	//for _, i := range interfaces {
	//	addrs, err := i.Addrs()
	//	if err != nil {
	//		continue
	//	}
	//
	//	for _, a := range addrs {
	//		fmt.Println(i, a)
	//	}
	//
	//	if i.Name == "eth1" {
	//		fmt.Println("using ", i)
	//		myIface = &i
	//	}
	//}

	var conn *net.UDPConn
	if conn, err = net.ListenMulticastUDP("udp4", myIface, addr); err != nil {
		panic(err)
	}

	log.Println("listening for packets")
	var buf [1024]byte
	for {
		if packetLength, remote, err := conn.ReadFromUDP(buf[:]); err == nil {
			packetString := string(buf[:packetLength])
			log.Println(remote.String(), strings.Replace(packetString, "\r\n", "", -1))
		}
	}

}
