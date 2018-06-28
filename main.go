package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	_ "github.com/heroku/x/hmetrics/onload"
)

type (
	registryInfo struct {
		Description []string
		HostAddr    string
		LastUpdate  time.Time
	}

	registry struct {
		sync.Mutex
		entries map[string]registryInfo
	}
)

var (
	theRegistry = registry{
		entries: make(map[string]registryInfo),
	}
)

func handlePost(w http.ResponseWriter, req *http.Request) {
	_, service := path.Split(req.URL.Path)

	nfo := registryInfo{
		HostAddr: req.RemoteAddr,
		LastUpdate: time.Now(),
	}
	lines := bufio.NewScanner(req.Body)
	for lines.Scan() {
		nfo.Description = append(nfo.Description, lines.Text())
	}
	if lines.Err() != nil {
		logrus.WithError(lines.Err()).Error("unable to read user input")
	}

	theRegistry.Lock()
	theRegistry.entries[service] = nfo
	theRegistry.Unlock()

	defer req.Body.Close()
}

func handleGet(w http.ResponseWriter, req *http.Request) {
	_, service := path.Split(req.URL.Path)

	w.Header().Set("Content-Type", "application/json, charset=utf-8")

	theRegistry.Lock()
	nfo, ok := theRegistry.entries[service]
	theRegistry.Unlock()

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	buf, err := json.Marshal(nfo)
	if err != nil {
		logrus.WithError(err).Error("unable to encode info as json")
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(buf)))
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(buf)
	if err != nil {
		logrus.WithError(err).Error("unable to send output to client")
	}
}

func secure(handler http.Handler, theUser, thePwd string) http.HandlerFunc {
	// TODO(improve it - safe for current usage)
	return func(w http.ResponseWriter, req *http.Request) {
		user, pwd, ok := req.BasicAuth()
		if !ok {
			http.Error(w, "authenticate", http.StatusUnauthorized)
			return
		}
		if !(user == theUser && pwd == thePwd) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		handler.ServeHTTP(w, req)
	}
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	port := os.Getenv("PORT")
	theUser := os.Getenv("THE_USER")
	thePwd := os.Getenv("THE_PASSWORD")

	if port == "" {
		logrus.Fatal("$PORT must be set")
	}

	if theUser == "" {
		logrus.Fatal("$THE_USER must be set (use heroku config)")
	}

	if thePwd == "" {
		logrus.Fatal("$THE_PASSWORD must be set (use heroku config)")
	}

	mux := http.NewServeMux()
	// totally not RESTful, but this is just a hack
	mux.HandleFunc("/add-services/", handlePost)
	mux.HandleFunc("/services/", handleGet)

	err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", port), secure(mux, theUser, thePwd))
	if err != nil {
		logrus.WithError(err).Error("error starting http server")
		os.Exit(1)
	}
	os.Exit(0)
}
