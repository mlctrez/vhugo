package vhugo

import (
	"encoding/json"
	"github.com/boltdb/bolt"
	"github.com/gocraft/web"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"net/http"
)

type ApiServer struct {
	router *web.Router
}

type WebContext struct{}

var indexPage = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>vhugo</title>
	<link href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.7/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-BVYiiSIFeK1dGmJRAkycuHAHRg32OmUcww7on3RYdg4Va+PmSTsz/K68vbdEjh4u" crossorigin="anonymous">
  </head>
  <body>
    <script src="https://ajax.googleapis.com/ajax/libs/jquery/1.12.4/jquery.min.js"></script>
    <script src="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.7/js/bootstrap.min.js" integrity="sha384-Tc5IQib027qvyjSMfHjOMaLkfuWVxZxUPnCJA7l2mCWNIpG9mGCD8wGNIcPD7Txa" crossorigin="anonymous"></script>
  </body>
</html>
`

func (w *WebContext) Index(rw web.ResponseWriter, req *web.Request) {
	rw.Write([]byte(indexPage))
}

type HttpOperation struct {
	Bucket   string
	Key      string
	Body     io.ReadCloser
	Response map[string]interface{}
}

func getPathParameter(req *web.Request, parameter string) string {
	if result, ok := req.PathParams[parameter]; ok {
		return result
	}
	return ""
}

func (h *HttpOperation) Get(tx *bolt.Tx) error {
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

func (h *HttpOperation) Post(tx *bolt.Tx) error {
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

func (h *HttpOperation) Delete(tx *bolt.Tx) error {
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

func (w *WebContext) Handle(rw web.ResponseWriter, req *web.Request) {
	h := &HttpOperation{
		Response: make(map[string]interface{}),
		Bucket:   getPathParameter(req, "bucket_id"),
		Key:      getPathParameter(req, "key_id"),
		Body:     req.Body,
	}

	rw.Header().Add("Content-Type", "application/json")

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
	json.NewEncoder(rw).Encode(h.Response)

}

func NewApiServer() *ApiServer {

	router := web.New(WebContext{})
	router.Get("/", (*WebContext).Index)
	router.Get("/bucket", (*WebContext).Handle)
	router.Get("/bucket/:bucket_id", (*WebContext).Handle)
	router.Get("/bucket/:bucket_id/:key_id", (*WebContext).Handle)
	router.Post("/bucket/:bucket_id", (*WebContext).Handle)
	router.Post("/bucket/:bucket_id/:key_id", (*WebContext).Handle)
	router.Delete("/bucket/:bucket_id", (*WebContext).Handle)
	router.Delete("/bucket/:bucket_id/:key_id", (*WebContext).Handle)

	apiServer := &ApiServer{router: router}
	return apiServer
}

func (a *ApiServer) ListenAndServe() {
	http.ListenAndServe("10.0.0.63:8999", a.router)
}
