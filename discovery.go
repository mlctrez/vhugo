package vhugo

import (
	"bytes"
	"net"
	"strings"
)

var uPnPAddress = "239.255.255.250:1900"

type DiscoveryRequest struct {
	Remote string
	Packet string
}

type DiscoveryResponse struct {
	Remote string
	Packet string
}

func listenUPnP() (listener func(), err error) {
	var addr *net.UDPAddr
	if addr, err = net.ResolveUDPAddr("udp4", uPnPAddress); err != nil {
		return
	}

	var conn *net.UDPConn
	if conn, err = net.ListenMulticastUDP("udp4", nil, addr); err != nil {
		return
	}

	listener = func() {
		var buf [1024]byte
		for {
			if packetLength, remote, err := conn.ReadFromUDP(buf[:]); err == nil {
				packetString := string(buf[:packetLength])
				if strings.Contains(packetString, "ST: urn:schemas-upnp-org:device:basic:1") {
					d := &DiscoveryRequest{Remote: remote.String(), Packet: packetString}
					encConn.Publish("upnp.discovery", d)
				}
			}
		}
	}

	return listener, nil
}

func (dg *DeviceGroup) HandleDiscoveryRequest(d *DiscoveryRequest) {
	if addr, err := net.ResolveUDPAddr("udp4", d.Remote); err == nil {
		if con, err := net.DialUDP("udp4", nil, addr); err == nil {
			defer con.Close()

			b := &bytes.Buffer{}
			disoveryResponseTemplate.Execute(b, dg)
			encConn.Publish("upnp.response", &DiscoveryResponse{Remote: d.Remote, Packet: string(b.Bytes())})

			con.Write(b.Bytes())
		}
	}
}
