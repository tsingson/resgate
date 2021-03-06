package server

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/jirenius/resgate/server/codec"
	"github.com/jirenius/resgate/server/httpapi"
	"github.com/jirenius/resgate/server/reserr"
)

var nullBytes = []byte("null")
var notFoundBytes []byte

func init() {
	out, err := json.Marshal(reserr.ErrNotFound)
	if err != nil {
		panic(err)
	}
	notFoundBytes = out
}

func (s *Service) initAPIHandler() {}

func (s *Service) apiHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}

	apiPath := s.cfg.APIPath

	switch r.Method {
	case "GET":
		// Redirect paths with trailing slash (unless it is only the APIPath)
		if len(path) > len(apiPath) && path[len(path)-1] == '/' {
			notFoundHandler(w, r)
			return
		}

		rid := httpapi.PathToRID(path, r.URL.RawQuery, apiPath)
		if !codec.IsValidRID(rid, true) {
			notFoundHandler(w, r)
			return
		}

		s.temporaryConn(w, r, func(c *wsConn, cb func(interface{}, error)) {
			c.GetHTTPResource(rid, apiPath, cb)
		})

	case "POST":
		// Redirect paths with trailing slash (unless it is only the APIPath)
		if len(path) > len(apiPath) && path[len(path)-1] == '/' {
			notFoundHandler(w, r)
			return
		}

		rid, action := httpapi.PathToRIDAction(path, r.URL.RawQuery, apiPath)
		if !codec.IsValidRID(rid, true) || !codec.IsValidRID(action, false) {
			notFoundHandler(w, r)
			return
		}

		// Try to parse the body
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			httpError(w, &reserr.Error{Code: reserr.CodeBadRequest, Message: "Error reading request body: " + err.Error()})
			return
		}

		var params json.RawMessage
		if strings.TrimSpace(string(b)) != "" {
			err = json.Unmarshal(b, &params)
			if err != nil {
				httpError(w, &reserr.Error{Code: reserr.CodeBadRequest, Message: "Error decoding request body: " + err.Error()})
				return
			}
		}

		s.temporaryConn(w, r, func(c *wsConn, cb func(interface{}, error)) {
			switch action {
			case "new":
				c.NewHTTPResource(rid, s.cfg.APIPath, params, func(href string, err error) {
					if err == nil {
						w.Header().Set("Location", href)
						w.WriteHeader(http.StatusCreated)
					}
					cb(nil, err)
				})
			default:
				c.CallResource(rid, action, params, cb)
			}
		})

	default:
		httpError(w, reserr.ErrMethodNotAllowed)
	}
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write(notFoundBytes)
}

func (s *Service) temporaryConn(w http.ResponseWriter, r *http.Request, cb func(*wsConn, func(interface{}, error))) {
	c := s.newWSConn(nil, r)
	if c == nil {
		httpError(w, reserr.ErrServiceUnavailable)
		return
	}

	done := make(chan struct{})
	rs := func(data interface{}, err error) {
		defer c.dispose()
		defer close(done)

		var out []byte
		if err != nil {
			httpError(w, err)
			return
		}

		if data != nil {
			out, err = json.Marshal(data)
			if err != nil {
				httpError(w, err)
				return
			}

			if !bytes.Equal(out, nullBytes) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Write(out)
				return
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
	c.Enqueue(func() {
		if s.cfg.HeaderAuth != nil {
			c.AuthResource(s.cfg.headerAuthRID, s.cfg.headerAuthAction, nil, func(result interface{}, err error) {
				cb(c, rs)
			})
		} else {
			cb(c, rs)
		}
	})
	<-done
}

func httpError(w http.ResponseWriter, err error) {
	rerr := reserr.RESError(err)
	out, err := json.Marshal(rerr)
	if err != nil {
		httpError(w, err)
		return
	}

	var code int
	switch rerr.Code {
	case reserr.CodeNotFound:
		fallthrough
	case reserr.CodeTimeout:
		code = http.StatusNotFound
	case reserr.CodeAccessDenied:
		code = http.StatusUnauthorized
	case reserr.CodeMethodNotAllowed:
		code = http.StatusMethodNotAllowed
	case reserr.CodeInternalError:
		code = http.StatusInternalServerError
	case reserr.CodeServiceUnavailable:
		code = http.StatusServiceUnavailable
	default:
		code = http.StatusBadRequest
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	w.Write(out)
}
