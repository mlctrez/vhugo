package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/mlctrez/vhugo/devicedb"
	"github.com/mlctrez/vhugo/hlog"
	"github.com/mlctrez/vhugo/natsserver"
	"github.com/mlctrez/vhugo/tmpl"
	"github.com/mlctrez/web"
)

type ApiServer struct {
	DB          *devicedb.DeviceDB
	DeviceGroup *devicedb.DeviceGroup
	NS          *natsserver.NatsServer
	logger      *hlog.HLog
}

func New(db *devicedb.DeviceDB, dg *devicedb.DeviceGroup, ns *natsserver.NatsServer, logger *log.Logger) *ApiServer {
	return &ApiServer{
		DB: db, DeviceGroup: dg,
		NS: ns, logger: hlog.New(logger, fmt.Sprintf("ApiServer-%d", dg.ServerPort)),
	}
}

type ApiContext struct {
	server *ApiServer
}

func (c *ApiContext) Setup(rw web.ResponseWriter, req *web.Request) {

	groupID := req.PathParams["groupID"]
	if groupID != c.server.DeviceGroup.GroupID {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	// TODO: correct content type here?
	if settings, err := c.server.DeviceGroup.Setup(); err == nil {
		rw.Write(settings)
	} else {
		rw.WriteHeader(http.StatusInternalServerError)
	}
}

func (c *ApiContext) Lights(rw web.ResponseWriter, req *web.Request) {
	virtualLights, err := c.server.DB.GetVirtualLights(c.server.DeviceGroup.GroupID)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(rw).Encode(virtualLights)
}

func (c *ApiContext) Light(rw web.ResponseWriter, req *web.Request) {

	lightID := req.PathParams["lightID"]
	if lightID == "" {
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	virtualLight, err := c.server.DB.GetVirtualLight(c.server.DeviceGroup.GroupID, lightID)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(rw).Encode(virtualLight)

}

func (c *ApiContext) LightState(rw web.ResponseWriter, req *web.Request) {
	lightID := req.PathParams["lightID"]
	if lightID == "" {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	groupID := c.server.DeviceGroup.GroupID

	virtualLight, err := c.server.DB.GetVirtualLight(groupID, lightID)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	sr := &devicedb.StateRequest{}
	json.NewDecoder(req.Body).Decode(sr)

	virtualLight.UpdateState(sr)

	ch := make(map[string]interface{})
	ch["groupID"] = groupID
	ch["lightID"] = lightID
	ch["stateRequest"] = sr

	c.server.NS.Publish("lightStateChange", ch)

	err = c.server.DB.UpdateVirtualLight(groupID, virtualLight)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	json.NewEncoder(rw).Encode(virtualLight)

}

func (c *ApiContext) DeleteLight(rw web.ResponseWriter, req *web.Request) {
	lightID := req.PathParams["lightID"]
	if lightID == "" {
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	groupID := c.server.DeviceGroup.GroupID
	err := c.server.DB.DeleteVirtualLight(groupID, lightID)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (a *ApiServer) Run(ctx context.Context) {

	router := web.New(ApiContext{})

	apiServerContext, cancel := context.WithCancel(ctx)

	subscription, err := a.NS.Subscribe("upnp.discovery", a.HandleDiscoveryRequest)
	if err != nil {
		a.logger.Println("Run Subscribe", err)
		cancel()
		return
	}
	defer subscription.Unsubscribe()

	router.Middleware(a.logger.LoggerMiddleware)

	router.Middleware(func(ctx *ApiContext, rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
		ctx.server = a
		next(rw, req)
	})

	router.Get("/api/upnp/:groupID/setup.xml", (*ApiContext).Setup)
	router.Get("/api/:userID/lights", (*ApiContext).Lights)
	router.Get("/api/:userID/lights/:lightID", (*ApiContext).Light)
	router.Put("/api/:userID/lights/:lightID/state", (*ApiContext).LightState)
	router.Delete("/api/:userID/lights/:lightID", (*ApiContext).DeleteLight)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.DeviceGroup.ServerIP, a.DeviceGroup.ServerPort),
		Handler: router,
	}

	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			a.logger.Println("ListenAndServe", err)
		}
		a.logger.Println("ListenAndServe exited")
		cancel()
	}()
	<-apiServerContext.Done()
	a.logger.Println("apiServerContext.Done()")
	server.Shutdown(ctx)
	return
}

func (a *ApiServer) HandleDiscoveryRequest(d *DiscoveryRequest) {
	if addr, err := net.ResolveUDPAddr("udp4", d.Remote); err == nil {
		if con, err := net.DialUDP("udp4", nil, addr); err == nil {
			defer con.Close()

			b := &bytes.Buffer{}
			tmpl.DisoveryResponseTemplate.Execute(b, a.DeviceGroup)
			a.NS.Publish("upnp.response", &DiscoveryResponse{Remote: d.Remote, Packet: string(b.Bytes())})

			con.Write(b.Bytes())
		}
	}
}

type DiscoveryRequest struct {
	Remote string
	Packet string
}

type DiscoveryResponse struct {
	Remote string
	Packet string
}
