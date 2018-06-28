package natsserver

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/mlctrez/vhugo/hlog"
	"github.com/nats-io/gnatsd/server"
	"github.com/nats-io/go-nats"
)

type NatsServer struct {
	opts      *server.Options
	server    *server.Server
	conn      *nats.Conn
	logger    *hlog.HLog
	loggerSub *nats.Subscription
	encConn   *nats.EncodedConn
}

type NatsPublisher interface {
	Publish(subject string, v interface{}) error
}

func New(opts *server.Options, logger *log.Logger) *NatsServer {
	return &NatsServer{opts: opts, logger: hlog.New(logger, "NatsServer")}
}

func (n *NatsServer) Shutdown() {
	if n == nil {
		return
	}

	n.logger.Println("Shutdown() entry")

	var err error
	if n.loggerSub != nil {
		n.logger.Println("Shutdown() loggerSub.Unsubscribe()")
		if err = n.loggerSub.Unsubscribe(); err != nil {
			n.logger.Println("loggerSub.Unsubscribe()", err)
		}
	}
	if n.encConn != nil {
		n.logger.Println("Shutdown() encConn.Close()")
		n.encConn.Close()
	}
	if n.conn != nil {
		n.logger.Println("Shutdown() conn.Close()")
		n.conn.Close()
	}
	if n.server != nil {
		n.logger.Println("Shutdown() server.Shutdown()")
		n.server.Shutdown()
	}
	n.logger.Println("Shutdown() complete")
}

func (n *NatsServer) Start(ctx context.Context) error {

	n.server = server.New(n.opts)

	go n.server.Start()

	if serverReady := n.server.ReadyForConnections(5 * time.Second); !serverReady {
		n.Shutdown()
		return errors.New("failed to start server")
	}

	var err error

	opts := nats.GetDefaultOptions()
	opts.Servers = []string{"nats://" + n.server.Addr().String()}

	if n.conn, err = opts.Connect(); err != nil {
		n.Shutdown()
		return err
	}
	if n.encConn, err = nats.NewEncodedConn(n.conn, nats.JSON_ENCODER); err != nil {
		n.Shutdown()
		return err
	}
	if n.loggerSub, err = n.conn.Subscribe(">", n.MessageLogger); err != nil {
		n.Shutdown()
		return err
	}

	mContext, cancel := context.WithCancel(ctx)
	go func() {
		<-mContext.Done()
		n.logger.Println("Context.Done()")
		n.Shutdown()
		cancel()
	}()

	return nil
}

func (n *NatsServer) Publish(subject string, v interface{}) error {
	return n.encConn.Publish(subject, v)
}

func (n *NatsServer) Subscribe(subject string, cb nats.Handler) (*nats.Subscription, error) {
	return n.encConn.Subscribe(subject, cb)
}

func (n *NatsServer) MessageLogger(msg *nats.Msg) {
	n.logger.Println(msg.Subject, string(msg.Data))
}
