package cas

import (
	"net/http"
	"time"

	"github.com/jmcarbo/gocas/authenticator"
	"github.com/jmcarbo/gocas/config"
	"github.com/jmcarbo/gocas/ticket"
	"github.com/jmcarbo/gocas/util"
	"gopkg.in/mgo.v2/bson"
)

func forbidden(w http.ResponseWriter, svc string, msg string) {
	lt := ticket.NewLoginTicket(svc)
	w.WriteHeader(http.StatusForbidden)
	lt.Serve(w, util.ResolveTemplate("login"), util.LoginRequestorData{
		Config:   config.Get(),
		Session:  util.LoginRequestorSession{Service: svc},
		Message:  util.LoginRequestorMessage{Type: "danger", Message: msg},
		ShowForm: true})
}

func loginRequestorHandler(w http.ResponseWriter, r *http.Request) {
	tgt, err := r.Cookie("CASTGC")
	svc := r.FormValue("service")
	renew, gateway := r.FormValue("renew"), r.FormValue("gateway")
	lt := ticket.NewLoginTicket(svc)

	tr := authenticator.AvailableAuthenticators["trust"]
	// Directly create session if trust authentication is enabled and succeeds
	if config.Get().TrustAuthentication == "always" {
		if tr != nil {
			auth, u := tr.Auth(r)
			if auth {
				tgt := ticket.NewTicketGrantingTicket(u, util.GetRemoteAddr(r.RemoteAddr))
				http.SetCookie(w, &http.Cookie{Name: "CASTGC", Value: tgt.Ticket})

				lt.Serve(w, util.ResolveTemplate("login"), util.LoginRequestorData{
					Config:  config.Get(),
					Session: util.LoginRequestorSession{Service: svc, Username: tgt.Username}})
				return
			}
		}
	}

	// The client sent us a TGT, do not display login form
	if err == nil && renew != "true" {
		var tkt ticket.TicketGrantingTicket
		util.GetPersistence("tgt").Find(bson.M{"_id": tgt.Value, "client_ip": util.GetRemoteAddr(r.RemoteAddr)}).One(&tkt)

		// TGT is valid
		if tgt.Value == tkt.Ticket && time.Now().Before(tkt.Validity) {
			if svc != "" {
				// Service ID was provided, generate ST and redirect
				st := ticket.NewServiceTicket(tkt.Ticket, svc, true)
				st.Serve(w, r)
				return
			} else {
				// No service ID, display successfull login screen
				lt.Serve(w, util.ResolveTemplate("login"), util.LoginRequestorData{
					Config:  config.Get(),
					Session: util.LoginRequestorSession{Service: svc, Username: tkt.Username}})
				return
			}
		} else {
			util.IncrementFailedLogin(r.RemoteAddr, "")
		}
	}

	// Gateway auth required pre-established SSO session or trust authentication
	if gateway == "true" && svc != "" && (config.Get().TrustAuthentication == "always" || config.Get().TrustAuthentication == "on-gateway") {
		if tr != nil {
			auth, u := tr.Auth(r)
			if auth {
				tgt := ticket.NewTicketGrantingTicket(u, util.GetRemoteAddr(r.RemoteAddr))
				http.SetCookie(w, &http.Cookie{Name: "CASTGC", Value: tgt.Ticket})
				st := ticket.NewServiceTicket(tgt.Ticket, svc, true)
				st.Serve(w, r)
				return
			}
		}

		w.WriteHeader(http.StatusForbidden)
		lt.Serve(w, util.ResolveTemplate("login"), util.LoginRequestorData{
			Config:  config.Get(),
			Message: util.LoginRequestorMessage{Type: "danger", Message: "This service requires a pre-established SSO session."}})
		return
	}

	// No ST, no TGT, display login form
	lt.Serve(w, util.ResolveTemplate("login"), util.LoginRequestorData{
		Config:   config.Get(),
		Session:  util.LoginRequestorSession{Service: svc},
		ShowForm: true})
}

func loginAcceptorHandler(w http.ResponseWriter, r *http.Request) {
	svc := r.FormValue("service")
	lt := r.FormValue("lt")

	var tkt ticket.LoginTicket
	util.GetPersistence("lt").Find(bson.M{"_id": lt}).One(&tkt)
	util.GetPersistence("lt").Remove(bson.M{"_id": tkt.Ticket})

	// LT is missing or is unknown
	if lt == "" || tkt.Ticket != lt {
		util.IncrementFailedLogin(r.RemoteAddr, "")
		forbidden(w, svc, "Form submission token was incorrect.")
		return
	}
	// LT has expired
	if tkt.Validity.Before(time.Now()) {
		util.IncrementFailedLogin(r.RemoteAddr, "")
		forbidden(w, svc, "Form submission token has expired.")
		return
	}
	// LT was created for another service
	if svc != tkt.Service {
		util.IncrementFailedLogin(r.RemoteAddr, "")
		forbidden(w, svc, "Form submission token reused in another context.")
		return
	}

	auth, u := authenticator.AvailableAuthenticators[config.Get().Authenticator].Auth(r)
	if !auth {
		util.IncrementFailedLogin(r.RemoteAddr, u)
		forbidden(w, svc, "The credential you provided were incorrect.")
		return
	}

	tgt := ticket.NewTicketGrantingTicket(u, util.GetRemoteAddr(r.RemoteAddr))
	http.SetCookie(w, &http.Cookie{Name: "CASTGC", Value: tgt.Ticket})

	// Session established and service required, create ST and redirect
	if svc != "" {
		st := ticket.NewServiceTicket(tkt.Ticket, svc, false)
		st.Serve(w, r)
		return
	}

	// SSO session opened, no service requested
	nlt := ticket.NewEmptyLoginTicket()
	nlt.Serve(w, util.ResolveTemplate("login"), util.LoginRequestorData{
		Config:  config.Get(),
		Session: util.LoginRequestorSession{Service: svc, Username: tgt.Username}})
}
