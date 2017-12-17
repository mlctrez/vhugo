package vhugo

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/boltdb/bolt"
)

type ApiServer struct {
	httpServer *http.Server
}

type WebContext struct{}

func Index(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "text/html")
	if indexBytes, err := ioutil.ReadFile("index.html"); err != nil {
		rw.WriteHeader(500)
		rw.Write([]byte(err.Error()))
	} else {
		rw.Write(indexBytes)
	}
}

type BucketOp struct {
	Bucket   string
	Key      string
	Body     io.ReadCloser
	Response map[string]interface{}
}

func getPathParameter(req *http.Request, parameter string) string {

	pathParts := strings.Split(req.RequestURI, "/")
	if len(pathParts) > 0 && pathParts[0] == "" {
		pathParts = pathParts[1:]
	}

	if len(pathParts) > 1 && "buckets" != pathParts[0] {
		return ""
	}

	switch parameter {
	case "bucket_id":
		if len(pathParts) > 1 {
			return pathParts[1]
		}
	case "key_id":
		if len(pathParts) > 2 {
			return pathParts[2]
		}
	}
	return ""
}

func (h *BucketOp) Get(tx *bolt.Tx) error {
	if h.Bucket != "" {
		var bucket *bolt.Bucket
		if bucket = tx.Bucket([]byte(h.Bucket)); bucket == nil {
			return errors.New("not found : " + h.Bucket)
		}

		if h.Key == "" {
			c := bucket.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				key := string(k)
				h.Response[key] = map[string]string{"key": "/bucket/" + h.Bucket + "/" + key}
			}
		} else {
			if kv := bucket.Get([]byte(h.Key)); kv != nil {
				rv := make(map[string]interface{})
				if err := json.Unmarshal(kv, &rv); err != nil {
					return err
				}
				h.Response = rv
			}
		}
	} else {
		c := tx.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			key := string(k)
			h.Response[key] = map[string]string{"url": "/bucket/" + key}
		}
	}
	return nil
}

func (h *BucketOp) Post(tx *bolt.Tx) error {
	if h.Bucket != "" {
		var bucket *bolt.Bucket
		var err error

		if bucket, err = tx.CreateBucketIfNotExists([]byte(h.Bucket)); err != nil {
			return err
		}

		if h.Key != "" {

			var bodyBytes []byte
			var err error

			if bodyBytes, err = ioutil.ReadAll(h.Body); err != nil {
				return err
			} else {
				payload := make(map[string]interface{})
				// checking for valid json syntax
				if err := json.Unmarshal(bodyBytes, &payload); err != nil {
					return err
				}

				bucket.Put([]byte(h.Key), bodyBytes)
			}
		}
	}
	return nil
}

func (h *BucketOp) Delete(tx *bolt.Tx) error {
	if h.Bucket != "" {
		var bucket *bolt.Bucket
		if h.Key != "" {
			if bucket = tx.Bucket([]byte(h.Bucket)); bucket != nil {
				return bucket.Delete([]byte(h.Key))
			}
		} else {
			return tx.DeleteBucket([]byte(h.Bucket))
		}
	}
	return nil
}

func (w *ApiServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if false {
		log.Println("serving ", req.Method, req.RequestURI)
	}
	if "/" == req.RequestURI {
		Index(rw, req)
		return
	}

	if !strings.HasPrefix(req.RequestURI, "/buckets") {
		return
	}

	h := &BucketOp{
		Response: make(map[string]interface{}),
		Bucket:   getPathParameter(req, "bucket_id"),
		Key:      getPathParameter(req, "key_id"),
		Body:     req.Body,
	}

	var err error

	switch req.Method {
	case "GET":
		err = boltDB.View(h.Get)
	case "POST":
		err = boltDB.Update(h.Post)
	case "DELETE":
		err = boltDB.Update(h.Delete)
	}

	if err != nil {
		h.Response["error"] = err.Error()
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(h.Response)

}

func NewApiServer(addr string) *ApiServer {
	apiServer := &ApiServer{}
	apiServer.httpServer = &http.Server{
		Handler: apiServer,
		Addr:    addr,
	}
	return apiServer
}

func (a *ApiServer) ListenAndServe() {
	a.httpServer.ListenAndServe()
}
