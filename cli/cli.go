package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kardianos/service"
	"github.com/mlctrez/servicego"
	"github.com/mlctrez/vhugo/apiserver"
	"github.com/mlctrez/vhugo/devicedb"
	"github.com/mlctrez/vhugo/hlog"
	"github.com/mlctrez/vhugo/natsserver"
	"github.com/mlctrez/vhugo/webapp"
	"github.com/mlctrez/web"
	"github.com/nats-io/gnatsd/server"
)

type serv struct {
	servicego.Defaults
	cancel func()
	ctx    context.Context
}

func (sv *serv) Start(s service.Service) error {
	sv.ctx, sv.cancel = context.WithCancel(context.Background())

	port := 19200

	if providedPort, err := strconv.Atoi(os.Getenv("PORT")); err == nil {
		port = providedPort
	}

	ip := os.Getenv("IP")
	if ip == "" {
		return fmt.Errorf("IP environment variable not set")
	}

	tlsHostName := os.Getenv("TLS_HOST")

	return Run(ip, port, tlsHostName, sv.ctx)
}

func (sv *serv) Stop(s service.Service) error {
	sv.cancel()
	time.Sleep(500 * time.Millisecond)
	<-sv.ctx.Done()
	return nil
}

func main() {
	servicego.Run(&serv{})
}

func Run(ip string, port int, tlsHostName string, mainContext context.Context) error {

	natsPort := port + 1
	apiPort := port + 2

	logger := log.New(os.Stdout, "", 0)
	web.Logger = logger

	ml := hlog.New(logger, "Main")

	opts := &server.Options{Host: ip, Port: natsPort, NoSigs: true}

	ns := natsserver.New(opts, logger)
	startError := ns.Start(mainContext)
	if startError != nil {
		return startError
	}

	deviceDB, err := devicedb.New("device.db", logger)
	if err != nil {
		return err
	}
	go func() {
		<-mainContext.Done()
		dbErr := deviceDB.Close()
		if dbErr != nil {
			logger.Println(dbErr)
		}
	}()

	webAddr := fmt.Sprintf("%s:%d", ip, port)

	app := webapp.New(deviceDB, ns, logger, tlsHostName)
	go app.Run(webAddr, mainContext)

	// TODO: configure the max number of device groups
	for i := apiPort; i < apiPort+4; i++ {

		groupID := fmt.Sprintf("group%d", i)
		ml.Println("checking initial device group", groupID)
		if _, err := deviceDB.GetDeviceGroup(groupID); err != nil {
			group := devicedb.NewDeviceGroup(groupID)
			group.ServerIP = ip
			group.ServerPort = i
			ml.Println("adding device group", group)
			err := deviceDB.AddDeviceGroup(group)
			if err != nil {
				ml.Println("error creating device group", err)
				return nil
			}
		}
	}

	deviceGroups, err := deviceDB.GetDeviceGroups()
	if err != nil {
		return err
	}
	for _, dg := range deviceGroups {
		deviceGroup := dg
		ml.Println("starting", deviceGroup)
		go apiserver.New(deviceDB, deviceGroup, ns, logger).Run(mainContext)
	}
	go listenUPnP(ns, hlog.New(logger, "ListenUPnP"), mainContext)
	return nil
}

func listenUPnP(ns natsserver.NatsPublisher, logger *hlog.HLog, ctx context.Context) {

	logger.Println("setting up uPnP listener")

	listenContext, cancel := context.WithCancel(ctx)
	defer cancel()

	var err error
	var addr *net.UDPAddr
	var conn *net.UDPConn

	if addr, err = net.ResolveUDPAddr("udp4", "239.255.255.250:1900"); err != nil {
		logger.Println("ResolveUDPAddr", err)
		return
	} else {
		if conn, err = net.ListenMulticastUDP("udp4", nil, addr); err != nil {
			logger.Println("ListenMulticastUDP", err)
			return
		}
	}
	defer conn.Close()

	go func() {
		var buf [1024]byte

		for {
			select {
			case <-listenContext.Done():
				logger.Println("exit in select")
				return
			default:
				if packetLength, remote, err := conn.ReadFromUDP(buf[:]); err == nil {
					packetString := string(buf[:packetLength])

					if strings.Contains(packetString, "ST: urn:schemas-upnp-org:device:basic:1") {
						d := &apiserver.DiscoveryRequest{Remote: remote.String(), Packet: packetString}
						ns.Publish("upnp.discovery", d)
					}
				}
				if err != nil {
					logger.Println("ReadFromUDP", err)
				}
			}
		}
	}()

	<-listenContext.Done()
	conn.Close()
	logger.Println("listenContext.Done()")

}
