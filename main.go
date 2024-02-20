package main

import (
	_ "embed"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
)

const AUTH_COOKIE_KEY = "__dead_simple_key"

//go:embed auth.html
var authPage []byte

type server struct {
	key     string
	forward *url.URL
	verbose bool
}

func (s server) handleAuth(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/__dead_simple_auth" || r.Method != "POST" {
		return false
	}
	err := r.ParseForm()
	if err != nil {
		return false
	}
	key := r.Form.Get("key")
	if key != s.key {
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:  AUTH_COOKIE_KEY,
		Value: s.key,
	})
	http.Redirect(w, r, "/", http.StatusFound)
	return true
}

func (s server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ok := s.handleAuth(w, r)
	if ok {
		return
	}

	cookie, err := r.Cookie(AUTH_COOKIE_KEY)
	if err != nil {
		if s.verbose {
			log.Println(err.Error())
		}
		w.Write(authPage)
		return
	}
	if cookie.Value != s.key {
		w.Write(authPage)
		return
	}

	newUrl := &url.URL{
		Scheme:   s.forward.Scheme,
		Host:     s.forward.Host,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	proxyReq, err := http.NewRequest(r.Method, newUrl.String(), r.Body)
	if err != nil {
		if s.verbose {
			log.Println(err.Error())
		}
		return
	}

	proxyReq.Header = make(http.Header)
	for h, val := range r.Header {
		proxyReq.Header[h] = val
	}

	res, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		if s.verbose {
			log.Println(err.Error())
		}
		return
	}
	defer res.Body.Close()

	for h, val := range res.Header {
		for _, v := range val {
			w.Header().Add(h, v)
		}
	}

	_, err = io.Copy(w, res.Body)
	if err != nil {
		if s.verbose {
			log.Println(err.Error())
		}
		return
	}
}

func main() {
	addr := flag.String("addr", ":8312", "the address to listen on")
	key := flag.String("key", "", "the authentication key, this can be any string")
	verbose := flag.Bool("verbose", false, "enable verbose logging")
	forwardStr := flag.String("forward", "", "the URL origin to forward requests to (ex. http://127.0.0.1:3000)")
	flag.Parse()

	if *key == "" {
		log.Fatal("you must specify a key, using -key")
	}
	if *forwardStr == "" {
		log.Fatal("you must specify a server to forward requests to, using -forward")
	}
	forward, err := url.Parse(*forwardStr)
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("listening on %s...\n", *addr)
	err = http.Serve(listener, server{
		key:     *key,
		forward: forward,
		verbose: *verbose,
	})
	if err != nil {
		log.Fatal(err)
	}
}
