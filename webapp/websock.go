package webapp

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mlctrez/web"
	"github.com/nats-io/go-nats"
)

func (w *WebContext) OnConnected(ws *websocket.Conn) {

	// no defer ws.Close() here to make sure client reconnects
	// in the event of a server restart

	webSocketcontext, cancel := context.WithCancel(w.App.ctx)
	defer cancel()

	app := w.App
	logger := app.logger
	address := ws.RemoteAddr().String()
	logger.Println("new client", address, "connected")

	defer func() {
		w.App.logger.Println("client", address, "sending close try again message")
		closeMessage := websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "shutting down or restarting")
		err := ws.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(time.Millisecond*500))
		if err != nil {
			w.App.logger.Println("client", address, "error sending close message", err)
		}
		err = ws.Close()
		if err != nil {
			w.App.logger.Println("client", address, "error closing socket", err)
		}
	}()

	subscription, err := app.Nats.Subscribe("lightStateChange", func(msg *nats.Msg) {
		err := ws.WriteMessage(websocket.TextMessage, msg.Data)
		if err != nil {
			logger.Println("Messages error writing to client", address, err)
			cancel()
		}
	})
	if err != nil {
		logger.Println("Nats.Subscribe", err)
		cancel()
		return
	}
	defer subscription.Unsubscribe()

	var running = true
	go func() {
		for running {
			md := make(map[string]interface{})
			if err := ws.ReadJSON(&md); err != nil {
				if err != io.EOF {
					logger.Println("receive error", address, err)
				}
				cancel()
				return
			}
			err := app.Nats.Publish("clientMessage", md)
			if err != nil {
				logger.Println("Publish clientMessage", err)
				cancel()
				return
			}
		}
	}()
	<-webSocketcontext.Done()
	running = false
	logger.Println("OnConnected exit", address)
}

func (w *WebContext) Messages(rw web.ResponseWriter, req *web.Request) {
	ws, err := w.App.upgrader.Upgrade(rw, req.Request, nil)
	if err != nil {
		w.App.logger.Println("Messages Upgrade", err)
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	w.OnConnected(ws)
}
