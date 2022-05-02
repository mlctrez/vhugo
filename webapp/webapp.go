package webapp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mlctrez/vhugo/devicedb"
	"github.com/mlctrez/vhugo/hlog"
	"github.com/mlctrez/vhugo/natsserver"
	"github.com/mlctrez/vhugo/static"
	"github.com/mlctrez/vhugo/tlsconfig"
	web "github.com/mlctrez/web"
)

type WebApp struct {
	logger        *hlog.HLog
	ctx           context.Context
	parentContext context.Context
	cancel        func()
	DB            *devicedb.DeviceDB
	Nats          *natsserver.NatsServer
	upgrader      websocket.Upgrader
	tlsHost       string
}

type WebContext struct {
	App *WebApp
}

func New(db *devicedb.DeviceDB, nats *natsserver.NatsServer, logger *log.Logger, tlsHostName string) *WebApp {
	return &WebApp{
		DB:       db,
		Nats:     nats,
		logger:   hlog.New(logger, "WebApp"),
		upgrader: websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024},
		tlsHost:  tlsHostName,
	}
}

type LightsResponse struct {
	Groups []string `json:"groups"`
	Lights []Light  `json:"lights"`
}

type Light struct {
	GroupID    string `json:"group_id"`
	LightID    string `json:"light_id"`
	Name       string `json:"name"`
	On         bool   `json:"on"`
	Brightness int32  `json:"brightness"`
}

type ByName []Light

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

func (w *WebContext) Lights(rw web.ResponseWriter, req *web.Request) {
	deviceGroups, err := w.App.DB.GetDeviceGroups()
	if err != nil {
		w.App.logger.Println("App.DB.GetDeviceGroups()", err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	lr := &LightsResponse{Lights: []Light{}, Groups: []string{}}
	for _, dg := range deviceGroups {
		lr.Groups = append(lr.Groups, dg.GroupID)
		lights, err := w.App.DB.GetVirtualLights(dg.GroupID)
		if err != nil {
			w.App.logger.Println("App.DB.GetVirtualLights(dg.GroupID)", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, l := range lights {
			lr.Lights = append(lr.Lights, Light{
				GroupID:    dg.GroupID,
				LightID:    devicedb.Sha(l.Name),
				Name:       l.Name,
				On:         l.State.On,
				Brightness: l.State.Bri,
			})
		}
	}
	sort.Sort(ByName(lr.Lights))
	json.NewEncoder(rw).Encode(lr)
}

type AddLightRequest struct {
	Name string
}

func (w *WebContext) AddLight(rw web.ResponseWriter, req *web.Request) {
	al := &AddLightRequest{}
	err := json.NewDecoder(req.Body).Decode(al)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	deviceGroups, err := w.App.DB.GetDeviceGroups()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	added := false
	for _, dg := range deviceGroups {
		lights, err := w.App.DB.GetVirtualLights(dg.GroupID)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		if len(lights) < 50 {
			light := devicedb.NewVirtualLight(al.Name)
			err = w.App.DB.UpdateVirtualLight(dg.GroupID, light)
			if err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
			added = true
			return
		}
	}
	if !added {
		rw.WriteHeader(http.StatusInternalServerError)
	}
}

func (w *WebContext) DeleteLight(rw web.ResponseWriter, req *web.Request) {
	groupID := req.PathParams["groupID"]
	lightID := req.PathParams["lightID"]
	err := w.App.DB.DeleteVirtualLight(groupID, lightID)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (w *WebContext) ChangeState(rw web.ResponseWriter, req *web.Request) {
	groupID := req.PathParams["groupID"]
	lightID := req.PathParams["lightID"]
	sr := &devicedb.StateRequest{}
	err := json.NewDecoder(req.Body).Decode(sr)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	virtualLight, err := w.App.DB.GetVirtualLight(groupID, lightID)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	virtualLight.UpdateState(sr)

	// TODO: consolidate the this function with apiserver.go:108
	ch := make(map[string]interface{})
	ch["groupID"] = groupID
	ch["lightID"] = lightID
	ch["stateRequest"] = sr

	w.App.Nats.Publish("lightStateChange", ch)

	err = w.App.DB.UpdateVirtualLight(groupID, virtualLight)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	l := &Light{
		Name:       virtualLight.Name,
		GroupID:    groupID,
		On:         virtualLight.State.On,
		Brightness: virtualLight.State.Bri,
	}
	json.NewEncoder(rw).Encode(l)
}

type WsMessage struct {
	MsgType string      `json:"msg_type"`
	Data    interface{} `json:"data"`
}

func (w *WebApp) Run(addr string, ctx context.Context) {

	w.logger.Println("Run() entry")
	webAppContext, cancel := context.WithCancel(ctx)
	defer cancel()

	w.ctx = webAppContext

	var config *tls.Config
	var err error
	if w.tlsHost != "" {
		config, err = tlsconfig.TlsConfig(w.tlsHost)
		if err != nil {
			return
		}
	}
	server := &http.Server{TLSConfig: config, Addr: addr}

	router := web.New(WebContext{})
	router.Middleware(func(a *WebContext, rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
		a.App = w
		next(rw, req)
	})
	router.Middleware(w.logger.LoggerMiddleware)

	router.Middleware(Static)

	router.Get("/api/messages", (*WebContext).Messages)
	router.Get("/api/lights", (*WebContext).Lights)
	router.Post("/api/lights", (*WebContext).AddLight)
	router.Post("/api/lights/:groupID/:lightID", (*WebContext).ChangeState)
	router.Delete("/api/lights/:groupID/:lightID", (*WebContext).DeleteLight)

	server.Handler = router
	go func() {
		if server.TLSConfig != nil {
			err = server.ListenAndServeTLS("", "")
		} else {
			w.logger.Println(fmt.Sprintf("web ui at http://%s", addr))
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			w.logger.Println("ListenAndServeTLS", err)
		}
		w.logger.Println("ListenAndServeTLS exit")
		cancel()
	}()

	<-webAppContext.Done()
	server.Shutdown(webAppContext)
	time.Sleep(50 * time.Millisecond)
	cancel()
	w.logger.Println("Run() exit")

}

func Static(w web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	if strings.HasPrefix(req.RequestURI, "/api") {
		next(w, req)
		return
	}
	http.FileServer(http.FS(static.Files)).ServeHTTP(w, req.Request)
}
