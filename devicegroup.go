package vhugo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type DeviceGroup struct {
	ServerIP   string
	ServerPort int
	GroupID    string
	UUID       string
	UU         string
}

type SettingsResponse struct {
	Payload string
}

type GenMap map[string]interface{}

var sparky = GenMap{
	"state": GenMap{
		"on":        true,
		"bri":       144,
		"hue":       13088,
		"sat":       212,
		"xy":        []float32{0.5128, 0.4147},
		"ct":        467,
		"alert":     "none",
		"effect":    "none",
		"colormode": "xy",
		"reachable": true,
	},
	"type":      "Extended color light",
	"name":      "sparky",
	"modelid":   "LCT001",
	"swversion": "66009461",
	"pointsymbol": GenMap{
		"1": "none",
		"2": "none",
		"3": "none",
		"4": "none",
		"5": "none",
		"6": "none",
		"7": "none",
		"8": "none",
	},
}

var lightList = GenMap{
	"sparky": sparky,
}

type VirtualLight struct {
	State       VirtualLightState `json:"state"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Modelid     string            `json:"modelid"`
	Swversion   string            `json:"swversion"`
	Pointsymbol map[string]string `json:"pointsymbol"`
}

type VirtualLightState struct {
	On        bool      `json:"on"`
	Bri       int32     `json:"bri"`
	Hue       int32     `json:"hue"`
	Sat       int32     `json:"sat"`
	Xy        []float32 `json:"xy"`
	Ct        int32     `json:"ct"`
	Alert     string    `json:"alert"`
	Effect    string    `json:"effect"`
	Colormode string    `json:"colormode"`
	Reachable bool      `json:"reachable"`
}

type StateRequest struct {
	On  bool  `json:"on"`
	Bri int32 `json:"bri"`
}

func (dg *DeviceGroup) ServeHTTP(rw http.ResponseWriter, rq *http.Request) {
	uri := rq.RequestURI

	log.Println("ServeHTTP", rq.RemoteAddr, rq.Method, uri)

	if rq.Method == "GET" {
		// /upnp/groupID/setup.xml
		if strings.HasPrefix(uri, "/upnp/") && strings.HasSuffix(uri, "/setup.xml") {

			b := &bytes.Buffer{}
			settingsTemplate.Execute(b, dg)
			encConn.Publish("http.settingsResponse", &SettingsResponse{Payload: string(b.Bytes())})

			rw.Write(b.Bytes())
			return
		}

		// /api/UserID/lights
		if strings.HasPrefix(uri, "/api/") && strings.HasSuffix(uri, "/lights") {
			json.NewEncoder(rw).Encode(&lightList)
			return
		}

		// /api/UserID/LightID
		if strings.HasPrefix(uri, "/api/") && strings.HasSuffix(uri, "/sparky") {
			json.NewEncoder(rw).Encode(&sparky)
			return
		}

	}

	if rq.Method == "PUT" {

		// /api/UserID/LightID/state

		if strings.HasPrefix(uri, "/api/") && strings.HasSuffix(uri, "/sparky/state") {

			bb, err := ioutil.ReadAll(rq.Body)
			if err != nil {
				log.Println("ERROR reading body")
				return
			}

			log.Println("BODY", string(bb))

			json.NewEncoder(rw).Encode(&sparky)

			return
		}
	}

}

func (dg *DeviceGroup) runServer() {

	encConn.Subscribe("upnp.discovery", dg.HandleDiscoveryRequest)

	addr := fmt.Sprintf("%s:%d", dg.ServerIP, dg.ServerPort)
	s := http.Server{Addr: addr}
	s.Handler = dg
	s.ListenAndServe()
}
