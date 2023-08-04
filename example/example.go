package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/vyks520/jsonrpc-ws-browser-client"
	"log"
	"net/http"
	"time"
)

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/jsonrpc-client", func(w http.ResponseWriter, r *http.Request) {
		rpc := jsonrpc.New(jsonrpc.RPCOpts{AllowCORS: true})

		rpc.HandleFunc(func(client *jsonrpc.RPCClient) {
			for true {
				res, err := client.Call("show-date", "001", time.Now().Format("2006-01-02 15:04:05"))
				if err != nil {
					fmt.Println(err)
					return
				}
				if res.Error != nil {
					fmt.Println(res.Error.Message)
					return
				}
				fmt.Printf("响应数据: %s\n", res.Result)
				time.Sleep(1 * time.Second)
			}
		})

		err := rpc.ClientWS(w, r)
		if err != nil {
			log.Printf("ws-client-client: %s", err.Error())
			return
		}
	})
	err := http.ListenAndServe(":8080", r)
	if err != nil {
		panic(err)
	}
}
