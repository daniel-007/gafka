package main

import (
	"net"
	"net/http"

	log "github.com/funkygao/log4go"
	"github.com/gorilla/mux"
)

type pubServer struct {
	maxClients int
	gw         *Gateway

	listener net.Listener
	server   *http.Server

	httpsServer *http.Server
	tlsListener net.Listener

	router *mux.Router

	exitCh <-chan struct{}
}

func newPubServer(httpAddr, httpsAddr string, maxClients int,
	gw *Gateway, exitCh <-chan struct{}) *pubServer {
	this := &pubServer{
		exitCh:     exitCh,
		router:     mux.NewRouter(),
		gw:         gw,
		maxClients: maxClients,
	}

	if httpAddr != "" {
		this.server = &http.Server{
			Addr:           httpAddr,
			Handler:        this.router,
			ReadTimeout:    0,       // FIXME
			WriteTimeout:   0,       // FIXME
			MaxHeaderBytes: 4 << 10, // should be enough
		}
	}

	if httpsAddr != "" {
		this.httpsServer = &http.Server{
			Addr:           httpAddr,
			Handler:        this.router,
			ReadTimeout:    0,       // FIXME
			WriteTimeout:   0,       // FIXME
			MaxHeaderBytes: 4 << 10, // should be enough
		}
	}

	return this
}

func (this *pubServer) Start() {
	var err error
	waited := false
	if this.server != nil {
		this.listener, err = net.Listen("tcp", this.server.Addr)
		if err != nil {
			panic(err)
		}

		this.listener = LimitListener(this.listener, this.maxClients)
		go this.server.Serve(this.listener)

		go this.waitExit()
		waited = true

		this.gw.wg.Add(1)
		log.Info("pub http server ready on %s", this.server.Addr)
	}

	if this.httpsServer != nil {
		this.tlsListener, err = this.gw.setupHttpsServer(this.httpsServer,
			this.gw.certFile, this.gw.keyFile)
		if err != nil {
			panic(err)
		}

		this.tlsListener = LimitListener(this.tlsListener, this.maxClients)
		go this.httpsServer.Serve(this.tlsListener)
		if !waited {
			go this.waitExit()
		}

		this.gw.wg.Add(1)
		log.Info("pub https server ready on %s", this.server.Addr)
	}

}

func (this *pubServer) Router() *mux.Router {
	return this.router
}

func (this *pubServer) waitExit() {
	for {
		select {
		case <-this.exitCh:
			// TODO https server

			// HTTP response will have "Connection: close"
			this.server.SetKeepAlivesEnabled(false)

			// avoid new connections
			if err := this.listener.Close(); err != nil {
				log.Error("listener close: %v", err)
			}

			this.listener = nil
			this.server = nil
			this.router = nil

			this.gw.wg.Done()
		}
	}
}
