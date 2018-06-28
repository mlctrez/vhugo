package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mlctrez/vhugo/apiserver"
	"github.com/mlctrez/vhugo/devicedb"
	"github.com/mlctrez/vhugo/hlog"
	"github.com/mlctrez/vhugo/natsserver"
	"github.com/mlctrez/vhugo/webapp"
	"github.com/mlctrez/web"
	"github.com/nats-io/gnatsd/server"
	"os/exec"
)

const HDHR_PLIST = "/Library/LaunchDaemons/com.silicondust.dvr.plist"

func main() {

	ip := flag.String("ip", "", "the ip to listen on (required)")
	port := flag.Int("port", 19200, "the starting port which is also the web interface")
	db := flag.String("ddb", "device.db", "device db path")
	flag.Parse()

	if *ip == "" {
		flag.Usage()
		log.Fatal("ip parameter must be provided")
	}

	// stop silicon dust service on mac which hogs port 1900
	if _, err := os.Stat(HDHR_PLIST); err == nil {
		cmd := exec.Command("/usr/bin/sudo", "launchctl", "unload", HDHR_PLIST)
		if _, err := cmd.CombinedOutput(); err != nil {
			panic(err)
		}
		// restart with
		// sudo launchctl load /Library/LaunchDaemons/com.silicondust.dvr.plist
		// sudo launchctl start /Library/LaunchDaemons/com.silicondust.dvr.plist
	}

	webAddr := fmt.Sprintf("%s:%d", *ip, *port)
	fmt.Println(webAddr)
	natsPort := *port + 1
	apiPort := *port + 2

	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	web.Logger = logger

	ml := hlog.New(logger, "Main")

	defer func() {
		ml.Println("exiting with ", runtime.NumGoroutine(), "go routines")
	}()

	opts := &server.Options{Host: *ip, Port: natsPort, NoSigs: true}

	mainContext, cancel := context.WithCancel(context.Background())
	defer func() {
		ml.Println("cancel()")
		cancel()
		time.Sleep(250 * time.Millisecond)
		ml.Println("cancel() complete")
	}()

	ns := natsserver.New(opts, logger)
	startError := ns.Start(mainContext)
	if startError != nil {
		ml.Println("error starting nats server", startError)
		return
	}

	deviceDB, err := devicedb.New(*db, logger)
	if err != nil {
		panic(err)
	}
	defer deviceDB.Close()

	go webapp.New(deviceDB, ns, logger).Run(webAddr, mainContext)

	// TODO: configure the max number of device groups
	for i := apiPort; i < apiPort+4; i++ {

		groupID := fmt.Sprintf("group%d", i)
		ml.Println("checking initial device group", groupID)
		if _, err := deviceDB.GetDeviceGroup(groupID); err != nil {
			group := devicedb.NewDeviceGroup(groupID)
			group.ServerIP = *ip
			group.ServerPort = i
			ml.Println("adding device group", group)
			err := deviceDB.AddDeviceGroup(group)
			if err != nil {
				ml.Println("error creating device group", err)
				return
			}
		}
	}

	deviceGroups, err := deviceDB.GetDeviceGroups()
	if err != nil {
		log.Println("deviceDB.GetDeviceGroups", err)
		return
	}
	for _, dg := range deviceGroups {
		deviceGroup := dg
		ml.Println("starting", deviceGroup)
		go apiserver.New(deviceDB, deviceGroup, ns, logger).Run(mainContext)
	}
	go listenUPnP(ns, hlog.New(logger, "ListenUPnP"), mainContext)

	signalChan := make(chan os.Signal, 1)
	signal.Reset()
	signal.Notify(signalChan, syscall.SIGKILL, syscall.SIGINT, syscall.SIGQUIT)
	ml.Println("listening for signals")
	ml.Println("signal:", <-signalChan)
}

func listenUPnP(ns natsserver.NatsPublisher, logger *hlog.HLog, ctx context.Context) {

	logger.Println("setting up uPnP listener")

	listenContext, _ := context.WithCancel(ctx)

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
