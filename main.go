package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"time"
	"sync"
)

var asset_path = flag.String("A", "/asset", "static assets path")
var server_address = flag.String("P", ":8080", "server listening address")
var sopcast_path = flag.String("S", "sp-sc-auth", "sopcast executable path")
var start_port = flag.Int("p", 5000, "first port to allocate")

type sop_channel struct {
	url                   string
	localport, playerport int
	cancel                context.CancelFunc
	open                  int
}

var channels map[string]*sop_channel

var ports [1000]bool
var port_mutex = &sync.Mutex{}

func free_port() int {
	port_mutex.Lock()
	for i:=0;i<1000;i++{
		if  ports[i]==false{
	ports[i]=true;port_mutex.Unlock();return i + *start_port}
	}
	port_mutex.Unlock()
	return 0
}

func release_port(port int){
port_mutex.Lock()
ports[port-*start_port]=false
port_mutex.Unlock()
}

func channel_close(url string) {
	c, ok := channels[url]
	if ok {
		c.cancel()
		release_port(c.localport)
		release_port(c.playerport)
		delete(channels, url)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	sop_url := "sop://" + r.RequestURI[5:]
	_, ok := channels[sop_url]

	if !ok {

		sop_localport := free_port()
		sop_playerport := free_port()
		ctx, cancel := context.WithCancel(context.Background())
		log.Printf("Starting sopcast %s %d %d",sop_url,sop_localport,sop_playerport)
		go func() {
			if err := exec.CommandContext(ctx, *sopcast_path, sop_url, strconv.Itoa(sop_localport), strconv.Itoa(sop_playerport)).Run(); err != nil {
				log.Printf("sopcast %s closed: %s", sop_url,err)
				channel_close(sop_url)
			}
		}()
		sc := sop_channel{
			url: sop_url, localport: sop_localport, playerport: sop_playerport, cancel: cancel}
		channels[sop_url] = &sc
		time.Sleep(time.Second * 30)

	}

	sop_c ,ok := channels[sop_url]
	if !ok {return}
	sop_c.open++
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/tv.asf", sop_c.playerport))
	if err != nil {
		log.Printf("Error connecting to sopcast client")
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
	log.Printf("End serving: %s", r.URL.Path)
	sop_c.open--
	if sop_c.open < 1 {
		log.Printf("Closing channel: %s", sop_c.url)
		channel_close(sop_c.url)
	}
}

func main() {
	flag.Parse()
	channels = make(map[string]*sop_channel)
	for i:=0;i<1000;i++{ports[i]=false}
	http.HandleFunc("/sop/", handler)
	http.Handle("/", http.FileServer(http.Dir(*asset_path)))
	log.Fatal(http.ListenAndServe(*server_address, logRequest(http.DefaultServeMux)))
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}
