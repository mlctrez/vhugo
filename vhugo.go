package vhugo

import (
	"encoding/json"
	"github.com/boltdb/bolt"
	"github.com/nats-io/gnatsd/server"
	"github.com/nats-io/nats"
	"github.com/satori/go.uuid"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

var natsServer *server.Server
var encConn *nats.EncodedConn
var boltDB *bolt.DB

var listenIP = ""

func logMessages() {
	opts := nats.DefaultOptions
	opts.Servers = []string{"nats://" + natsServer.GetListenEndpoint()}
	nc, err := opts.Connect()
	if err != nil {
		panic(err)
	}

	cb := make(chan *nats.Msg, 10)
	nc.ChanSubscribe(">", cb)

	for !nc.IsClosed() {
		select {
		case m := <-cb:
			log.Println(m.Subject, string(m.Data))
		}
	}
}

func startNats() {

	// TODO : make server and port configurable
	natsOptions := &server.Options{}

	natsServer = server.New(natsOptions)

	go natsServer.Start()

	// follows https://github.com/nats-io/gnatsd/blob/master/test/test.go#L84
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {

		addr := natsServer.GetListenEndpoint()
		if addr == "" {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		opts := nats.DefaultOptions
		opts.Servers = []string{"nats://" + addr}

		conn, err := opts.Connect()
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		encConn, err = nats.NewEncodedConn(conn, nats.JSON_ENCODER)
		if err != nil {
			panic(err)
		}

		log.Println("started nats server")
		return
	}
	panic("unable to start nats server")
}

func getDeviceGroups() (deviceGroups []*DeviceGroup, err error) {
	deviceGroups = make([]*DeviceGroup, 0)

	err = boltDB.Update(func(tx *bolt.Tx) (err error) {
		var b *bolt.Bucket
		if b, err = tx.CreateBucketIfNotExists([]byte("DeviceGroups")); err != nil {
			return err
		}

		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			_ = k
			dg := &DeviceGroup{}
			if err = json.Unmarshal(v, dg); err == nil {
				deviceGroups = append(deviceGroups, dg)
			} else {
				log.Println("skipping invalid device group json")
			}
		}

		return nil
	})
	return deviceGroups, err
}

func UpdateDB(fn func(tx *bolt.Tx) (err error)) {
	if err := boltDB.Update(fn); err != nil {
		log.Fatal(err)
	}
}

// Get preferred outbound ip of this machine
func GetOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().String()
	idx := strings.LastIndex(localAddr, ":")

	return localAddr[0:idx]
}

func Run() {
	log.SetOutput(os.Stdout)

	listenIP = GetOutboundIP()
	log.Println("listening on address", listenIP)

	var err error

	boltDB, err = bolt.Open("device.db", 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	defer boltDB.Close()

	s := NewApiServer(listenIP + ":8999")
	go s.ListenAndServe()

	deviceGroups, err := getDeviceGroups()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("found %d device groups", len(deviceGroups))

	if len(deviceGroups) == 0 {

		uuID := uuid.NewV1()
		parts := strings.Split(uuID.String(), "-")
		dg := &DeviceGroup{ServerIP: listenIP, ServerPort: 9000, GroupID: "groupOne", UUID: uuID.String(), UU: parts[len(parts)-1]}

		UpdateDB(func(tx *bolt.Tx) (e error) {
			if val, err := json.Marshal(dg); err == nil {
				e = tx.Bucket([]byte("DeviceGroups")).Put([]byte(dg.GroupID), val)
			}
			return e
		})
	}

	startNats()
	go logMessages()

	for _, dg := range deviceGroups {
		go dg.runServer()
	}

	listener, err := listenUPnP()
	if err != nil {
		log.Fatal(err)
	}

	listener()
}
