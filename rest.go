package main

import (
	"fmt"
	"net/http"

	"github.com/jmcarbo/gocas/authenticator"
	"github.com/jmcarbo/gocas/config"
	"github.com/jmcarbo/gocas/ticket"
	"github.com/jmcarbo/gocas/util"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2/bson"
)

func restGetTicketGrantingTicketHandler(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("username") == "" || r.FormValue("password") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	auth, u := authenticator.AvailableAuthenticators[config.Get().Authenticator].Auth(r)
	if !auth {
		util.IncrementFailedLogin(r.RemoteAddr, "")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	tgt := ticket.NewTicketGrantingTicket(u, util.GetRemoteAddr(r.RemoteAddr))
	w.Header().Add("Location", fmt.Sprintf("%s%s/%s", config.Get().Url, r.RequestURI, tgt.Ticket))
	w.WriteHeader(http.StatusCreated)
}

func restGetServiceTicketHandler(w http.ResponseWriter, r *http.Request) {
	tgt := mux.Vars(r)["ticket"]
	svc := r.FormValue("service")

	if tgt == "" || svc == "" {
		util.IncrementFailedLogin(r.RemoteAddr, "")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var tkt ticket.TicketGrantingTicket
	util.GetPersistence("tgt").Find(bson.M{"_id": tgt, "client_ip": util.GetRemoteAddr(r.RemoteAddr)}).One(&tkt)
	if tgt != tkt.Ticket {
		util.IncrementFailedLogin(r.RemoteAddr, "")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	st := ticket.NewServiceTicket(tkt.Ticket, svc, true)
	if !st.Validate() {
		util.IncrementFailedLogin(r.RemoteAddr, "")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	w.Write([]byte(st.Ticket))
}

func restLogoutHandler(w http.ResponseWriter, r *http.Request) {
	tgt := mux.Vars(r)["ticket"]

	util.GetPersistence("tgt").Remove(bson.M{"_id": tgt, "client_ip": util.GetRemoteAddr(r.RemoteAddr)})
}
