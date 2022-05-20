package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/kelseyhightower/envconfig"
	"log"
	"net/http"
	"strconv"
	"sync"
)

type Config struct {
	HttpsListenPort int    `envconfig:"HTTPS_PORT" default:"443"`
	HttpListenPort  int    `envconfig:"HTTP_PORT" default:"80"`
	TlsCert         string `envconfig:"TLS_CERT" default:"/usr/src/app/pki/tls.crt"`
	TlsKey          string `envconfig:"TLS_KEY" default:"/usr/src/app/pki/tls.key"`
}

func rootHandler(w http.ResponseWriter, req *http.Request) {
	remoteAddress := req.RemoteAddr
	fwdAddress := req.Header.Get("X-Forwarded-For")
	resp := "Got / request from Connecting from %v"
	if len(fwdAddress) > 0 {
		resp = fmt.Sprintf(resp+", forwarded by %v!\n", fwdAddress, remoteAddress)
	} else {
		resp = fmt.Sprintf(resp, remoteAddress)
	}
	fmt.Fprintf(w, resp)
}

func headerdumpHandler(w http.ResponseWriter, r *http.Request) {
	for k, v := range r.Header {
		fmt.Fprintf(w, "%v: %v\n", k, v)
	}
	// https://cs.opensource.google/go/go/+/refs/tags/go1.20.1:src/net/http/request.go;l=157-158;drc=fd0c0db4a411eae0483d1cb141e801af401e43d3
	fmt.Fprintf(w, "Host: [%v]\n", r.Host)
}

func healthCheckHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "ok\n")
}

func setupHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", healthCheckHandler)
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/headers", headerdumpHandler)
}

func parseTlsConfig(c Config) *tls.Config {
	cer, err := tls.LoadX509KeyPair(c.TlsCert, c.TlsKey)
	if err != nil {
		log.Printf("Failed to load keypair [%s, %s]: %s", c.TlsCert, c.TlsKey, err)
		return nil
	}
	return &tls.Config{Certificates: []tls.Certificate{cer}}
}

func newHttpServer(c Config) *http.Server {
	return &http.Server{
		Addr: ":" + strconv.Itoa(c.HttpListenPort),
	}
}

func newHttpsServer(c Config) *http.Server {
	return &http.Server{
		Addr:      ":" + strconv.Itoa(c.HttpsListenPort),
		TLSConfig: parseTlsConfig(c),
	}
}

func main() {

	mux := http.NewServeMux()
	var config Config
	err := envconfig.Process("", &config)
	if err != nil {
		log.Fatal(err.Error())
	}
	setupHandlers(mux)

	httpsServer := newHttpsServer(config)
	httpsServer.Handler = mux
	httpServer := newHttpServer(config)
	httpServer.Handler = mux

	wg := new(sync.WaitGroup)
	wg.Add(2)

	go func() {
		log.Printf("Starting listening for incoming HTTPS requests on %v", httpsServer.Addr)
		err := httpsServer.ListenAndServeTLS("", "")
		if errors.Is(err, http.ErrServerClosed) {
			log.Printf("https responder closed\n")
		} else if err != nil {
			log.Printf("error listening for https: %s\n", err)
		}
		wg.Done()
	}()
	go func() {
		log.Printf("Starting listening for incoming HTTP requests on %v", httpServer.Addr)
		err := httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			log.Printf("http responder closed\n")
		} else if err != nil {
			log.Fatalf("error listening for http: %s\n", err)
		}
		wg.Done()
	}()
	wg.Wait()
}
