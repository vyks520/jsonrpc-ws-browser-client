# jsonrpc-ws-browser-client

jsonrpc-ws-browser-client是一个以浏览器为JSON-RPC 2.0服务端的客户端实现，用于后端向浏览器进行主动调用，例如聊天信息推送等。  

本项目在基于 [ybbus/jsonrpc](https://github.com/ybbus/jsonrpc) 改造

需配合前端项目使用  

前端项目地址：[vyks520/jsonrpc-ws-browser-server](https://github.com/vyks520/jsonrpc-ws-browser-server)  

##  Installation
```
$ go get -u github.com/vyks520/jsonrpc-ws-browser-client
```

##  Getting started  ``example.go``
[前端示例](https://github.com/vyks520/jsonrpc-ws-browser-server/blob/main/example/example.html)

```go
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
```

##  Call 方法
用于向前端服务器端点发送JSON-RPC请求
```go
rpc := jsonrpc.New(jsonrpc.RPCOpts{AllowCORS: true})

rpc.HandleFunc(func(client *jsonrpc.RPCClient) {
    for true {
        res, err := client.Call("method-name", "参数1", "参数2","...参数n")
        if err != nil {
            fmt.Println(err)
            return
        }
        if res.Error != nil {
            fmt.Println(res.Error.Message)
            return
        }
        fmt.Printf("响应数据: %s\n", res.Result)
    }
})
```
##  CallRaw 方法
CallRaw 方法与Call类似，但是没有对参数进行Params处理，发起的请求与提供的RPCRequest一致。
```go
rpc := jsonrpc.New(jsonrpc.RPCOpts{AllowCORS: true})

rpc.HandleFunc(func(client *jsonrpc.RPCClient) {
    res, err := client.CallRaw(jsonrpc.RPCRequest{
        Method: "method-name",
        Params: [2]string{"参数1", "参数2"},
        ID:     "123",
    })
})
```

##  CallFor 
CallFor是一个很方便的方法，可以直接传入执行结果的结构类型，直接返回数据。
```go
package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/vyks520/jsonrpc-ws-browser-client"
	"log"
	"net/http"
	"time"
)

type Person struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type PersonResData struct {
	jsonrpc.RPCResponse
	Result Person `json:"result,omitempty"`
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/jsonrpc-client", func(w http.ResponseWriter, r *http.Request) {
		rpc := jsonrpc.New(jsonrpc.RPCOpts{AllowCORS: true})
		rpc.HandleFunc(func(client *jsonrpc.RPCClient) {
			for true {
				res := PersonResData{}
				err := client.CallFor(&res, "getPersonInfo", "001")
				if err != nil {
					fmt.Println(err)
					return
				}
				if res.Error != nil {
					fmt.Println(res.Error.Message)
					return
				}
				user := res.Result
				fmt.Println(fmt.Sprintf("ID: %s, 姓名: %s, 年龄: %d", user.Id, user.Name, user.Age))
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
```
##  CallBatch
CallBatch用于向服务器批处理发送JSON-RPC请求，响应数据结构为RPCResponses



## RPC调用参数示例

```go
package main

import (
	"github.com/gorilla/mux"
	"github.com/vyks520/jsonrpc-ws-browser-client"
	"log"
	"net/http"
)

type Person struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Country string `json:"country"`
}

type Drink struct {
	Name        string   `json:"name"`
	Ingredients []string `json:"ingredients"`
}

var (
	person = Person{
		Name:    "Alex",
		Age:     35,
		Country: "Germany",
	}
	drink = Drink{
		Name:        "Cuba Libre",
		Ingredients: []string{"rum", "cola"},
	}
)

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/jsonrpc-client", func(w http.ResponseWriter, r *http.Request) {
		rpc := jsonrpc.New(jsonrpc.RPCOpts{AllowCORS: true})

		rpc.HandleFunc(func(client *jsonrpc.RPCClient) {
			_, _ = client.Call("missingParam")
			// {"method":"missingParam"}

			_, _ = client.Call("nullParam", nil)
			//{"method":"nullParam","params":[null]}

			_, _ = client.Call("boolParam", true)
			//{"method":"boolParam","params":[true]}

			_, _ = client.Call("boolParams", true, false, true)
			// {"method":"boolParams","params":[true,false,true]}

			_, _ = client.Call("stringParam", "Alex")
			// {"method":"stringParam","params":["Alex"]}

			_, _ = client.Call("stringParams", "JSON", "RPC")
			// {"method":"stringParams","params":["JSON","RPC"]}

			_, _ = client.Call("numberParam", "123")
			// {"method":"numberParam","params":[123]}

			_, _ = client.Call("numberParams", 123, 321)
			// {"method":"numberParams","params":[123,321]}

			_, _ = client.Call("floatParam", 1.23)
			// {"method":"floatParam","params":[1.23]}

			_, _ = client.Call("floatParams", 1.23, 3.21)
			// {"method":"floatParams","params":[1.23,3.21]}

			_, _ = client.Call("manyParams", "Alex", 35, true, nil, 2.34)
			// {"method":"manyParams","params":["Alex",35,true,null,2.34]}

			_, _ = client.Call("singlePointerToStruct", &person)
			// {"method":"singlePointerToStruct","params":{"name":"Alex","age":35,"country":"Germany"}}

			_, _ = client.Call("multipleStructs", &person, &drink)
			// {"method":"multipleStructs","params":[// {"name":"Alex","age":35,"country":"Germany"},{"name":"Cuba Libre","ingredients":["rum","cola"]}]}

			_, _ = client.Call("singleStructInArray", []*Person{&person})
			// {"method":"singleStructInArray","params":[{"name":"Alex","age":35,"country":"Germany"}]}

			_, _ = client.Call("namedParameters", map[string]interface{}{
				"name": "Alex",
				"age":  35,
			})
			// {"method":"namedParameters","params":{"age":35,"name":"Alex"}}

			_, _ = client.Call("anonymousStruct", struct {
				Name string `json:"name"`
				Age  int    `json:"age"`
			}{"Alex", 33})
			// {"method":"anonymousStructWithTags","params":{"name":"Alex","age":33}}

			_, _ = client.Call("structWithNullField", struct {
				Name    string  `json:"name"`
				Address *string `json:"address"`
			}{"Alex", nil})
			// {"method":"structWithNullField","params":{"name":"Alex","address":null}}
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
```

