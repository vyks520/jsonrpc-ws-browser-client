package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"
)

const (
	jsonrpcVersion = "2.0"
)

// RPCClient sends JSON-RPC requests over HTTP to the provided JSON-RPC backend.
//
// RPCClient is created using the factory function NewClient().
type RPCClient interface {
	// ClientWS processes JSON-RPC 2.0 requests via Gorilla WebSocket.
	ClientWS(w http.ResponseWriter, r *http.Request) error

	Close() error

	// Call is used to send a JSON-RPC request to the server endpoint.
	Call(method string, params ...interface{}) (*RPCResponse, error)

	// CallRaw is like Call() but without magic in the requests.Params field.
	// The RPCRequest object is sent exactly as you provide it.
	// See docs: NewRequest, RPCRequest, Params()
	//
	// It is recommended to first consider Call() and CallFor()
	CallRaw(request *RPCRequest) (*RPCResponse, error)

	// CallFor is a very handy function to send a JSON-RPC request to the server endpoint
	// and directly specify an object to store the response.
	//
	// out: will store the unmarshaled object, if request was successful.
	// should always be provided by references. can be nil even on success.
	// the behaviour is the same as expected from json.Unmarshal()
	//
	// method and params: see Call() function
	//
	// if the request was not successful (network, http error) or the rpc response returns an error,
	// an error is returned. if it was an JSON-RPC error it can be casted
	// to *RPCError.
	CallFor(out interface{}, method string, params ...interface{}) error

	// CallBatch invokes a list of RPCRequests in a single batch request.
	//
	// Most convenient is to use the following form:
	// CallBatch(ctx, RPCRequests{
	//   NewRequest("myMethod1", 1, 2, 3),
	//   NewRequest("myMethod2", "Test"),
	// })
	CallBatch(requests RPCRequests) (RPCResponses, error)

	// CallBatchFor is a very handy function to send a JSON-RPC requests to the server endpoint
	// and directly specify an object to store the response.
	//
	// out: will store the unmarshaled object, if request was successful.
	// should always be provided by references. can be nil even on success.
	// the behaviour is the same as expected from json.Unmarshal()
	// CallBatchFor(out, RPCRequests{
	//   &RPCRequest{
	//     ID: 123,            // this won't be replaced in CallBatchRaw
	//     JSONRPC: "wrong",   // this won't be replaced in CallBatchRaw
	//     Method: "myMethod1",
	//     Params: []int{1},   // there is no magic, be sure to only use array or object
	//   },
	//   &RPCRequest{
	//     ID: 612,
	//     JSONRPC: "2.0",
	//     Method: "myMethod2",
	//     Params: Params("Alex", 35, true), // you can use helper function Params() (see doc)
	//   },
	// })
	CallBatchFor(out interface{}, requests RPCRequests) error
}

type RPCRequest struct {
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      string      `json:"id"`
	JSONRPC string      `json:"jsonrpc"`
}

func NewRequest(method string, params ...interface{}) *RPCRequest {
	request := &RPCRequest{
		ID:      uuid.NewV4().String(),
		Method:  method,
		Params:  Params(params...),
		JSONRPC: jsonrpcVersion,
	}
	return request
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return strconv.Itoa(e.Code) + ": " + e.Message
}

type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      string      `json:"id"`
}

type rpcResponseID struct {
	ID string `json:"id"`
}

type RPCResponses []*RPCResponse

type RPCRequests []*RPCRequest

type IDAwaiterMethod func(res []byte, e error)

type IDAwaiter struct {
	sync.RWMutex
	methods map[string]IDAwaiterMethod
}

type rpcClient struct {
	Conn            *websocket.Conn
	upgrader        *websocket.Upgrader
	idAwaiter       IDAwaiter
	responseTimeout time.Duration
	logger          *log.Logger
}

type RPCClientOpts struct {
	Conn            *websocket.Conn
	AllowCORS       bool
	ResponseTimeout time.Duration
	Logger          *log.Logger
}

func newError(err interface{}) error {
	return errors.New(fmt.Sprintf("jsonrpc-client: %s", fmt.Sprint(err)))
}

func IsJsonArrBytes(b []byte) (bool, error) {
	textLen := len(b)
	err := errors.New("too many invalid characters or invalid 'json text'")
	for i := 0; i < textLen; i++ {
		if i > 100 {
			return false, err
		}
		switch b[i] {
		case 91: // [
			return true, nil
		case 123: // {
			return false, nil
		}
	}
	return false, err
}

// Params is a helper function that uses the same parameter syntax as Call().
// But you should consider to always use NewRequest() instead.
//
// e.g. to manually create an RPCRequest object:
// request := &RPCRequest{
//   Method: "myMethod",
//   Params: Params("Alex", 35, true),
// }
//
// same with new request:
// request := NewRequest("myMethod", "Alex", 35, true)
//
// If you know what you are doing you can omit the Params() call but potentially create incorrect rpc requests:
// request := &RPCRequest{
//   Method: "myMethod",
//   Params: 2, <-- invalid since a single primitive value must be wrapped in an array --> no magic without Params()
// }
//
// correct:
// request := &RPCRequest{
//   Method: "myMethod",
//   Params: []int{2}, <-- valid since a single primitive value must be wrapped in an array
// }
func Params(params ...interface{}) interface{} {
	var finalParams interface{}

	// if params was nil skip this and p stays nil
	if params != nil {
		switch len(params) {
		case 0: // no parameters were provided, do nothing so finalParam is nil and will be omitted
		case 1: // one param was provided, use it directly as is, or wrap primitive types in array
			if params[0] != nil {
				var typeOf reflect.Type

				// traverse until nil or not a pointer type
				for typeOf = reflect.TypeOf(params[0]); typeOf != nil && typeOf.Kind() == reflect.Ptr; typeOf = typeOf.Elem() {
				}

				if typeOf != nil {
					// now check if we can directly marshal the type or if it must be wrapped in an array
					switch typeOf.Kind() {
					// for these types we just do nothing, since value of p is already unwrapped from the array params
					case reflect.Struct:
						finalParams = params[0]
					case reflect.Array:
						finalParams = params[0]
					case reflect.Slice:
						finalParams = params[0]
					case reflect.Interface:
						finalParams = params[0]
					case reflect.Map:
						finalParams = params[0]
					default: // everything else must stay in an array (int, string, etc)
						finalParams = params
					}
				}
			} else {
				finalParams = params
			}
		default: // if more than one parameter was provided it should be treated as an array
			finalParams = params
		}
	}

	return finalParams
}

func NewClient(opts RPCClientOpts) RPCClient {
	client := &rpcClient{
		Conn:      opts.Conn,
		idAwaiter: IDAwaiter{methods: map[string]IDAwaiterMethod{}},
	}

	// 超时小于5秒无效
	if opts.ResponseTimeout > 5*time.Second {
		client.responseTimeout = opts.ResponseTimeout
	} else {
		// 默认超时时间15秒
		client.responseTimeout = 15 * time.Second
	}

	client.upgrader = &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return opts.AllowCORS },
	}

	if opts.Logger != nil {
		client.logger = opts.Logger
	} else {
		client.logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	return client
}

func (client *rpcClient) Close() error {
	return client.Conn.Close()
}

func (client *rpcClient) doCall(out interface{}, request *RPCRequest) error {
	request.JSONRPC = jsonrpcVersion
	if client.Conn == nil {
		return newError("websocket.Conn Not created")
	}
	err := client.Conn.WriteJSON(request)
	if err != nil {
		return newError(err.Error())
	}

	var rpcResData []byte
	resChannel := make(chan bool)

	// 不需要返回结果则不执行响应处理逻辑
	if out != nil {
		client.idAwaiter.Lock()
		client.idAwaiter.methods[request.ID] = func(res []byte, e error) {
			client.idAwaiter.RLock()
			_, ok := client.idAwaiter.methods[request.ID]
			client.idAwaiter.RUnlock()
			if ok {
				client.idAwaiter.Lock()
				delete(client.idAwaiter.methods, request.ID)
				client.idAwaiter.Unlock()
			}
			err = e
			rpcResData = res
			resChannel <- true
		}
		client.idAwaiter.Unlock()

		go func() {
			time.Sleep(client.responseTimeout)
			client.idAwaiter.RLock()
			_, ok := client.idAwaiter.methods[request.ID]
			client.idAwaiter.RUnlock()
			if ok {
				client.idAwaiter.Lock()
				delete(client.idAwaiter.methods, request.ID)
				client.idAwaiter.Unlock()
				err = newError("request timed out")
				resChannel <- true
			}
		}()
		<-resChannel
		if err != nil {
			return err
		}
		err = json.Unmarshal(rpcResData, out)
		if err != nil {
			return newError(err.Error())
		}
		return err
	}
	return nil
}

func (client *rpcClient) Call(method string, params ...interface{}) (*RPCResponse, error) {
	request := NewRequest(method, params...)
	rpcRes := &RPCResponse{}
	err := client.doCall(rpcRes, request)
	return rpcRes, err
}

func (client *rpcClient) CallRaw(request *RPCRequest) (*RPCResponse, error) {
	rpcRes := &RPCResponse{}
	err := client.doCall(rpcRes, request)
	return rpcRes, err
}

func (client *rpcClient) CallFor(out interface{}, method string, params ...interface{}) error {
	request := NewRequest(method, params...)
	err := client.doCall(out, request)
	return err
}

func (client *rpcClient) doBatchCall(out interface{}, requests []*RPCRequest) error {
	for _, req := range requests {
		req.JSONRPC = jsonrpcVersion
	}
	if len(requests) == 0 {
		return newError("empty request list")
	}
	if client.Conn == nil {
		return newError("websocket.Conn Not created")
	}
	err := client.Conn.WriteJSON(requests)
	if err != nil {
		return newError(err.Error())
	}

	// 不需要返回结果则不执行响应处理逻辑
	// JSON-RPC 2.0 规范: 若批量调用没有需要返回的响应对象，则服务端不需要返回任何结果且必须不能返回一个空数组给客户端。
	if out != nil {
		var rpcResData []byte
		resChannel := make(chan bool)

		method := func(res []byte, e error) {
			for _, req := range requests {
				client.idAwaiter.RLock()
				_, ok := client.idAwaiter.methods[req.ID]
				client.idAwaiter.RUnlock()
				if ok {
					client.idAwaiter.Lock()
					delete(client.idAwaiter.methods, req.ID)
					client.idAwaiter.Unlock()
				}
			}
			err = e
			rpcResData = res
			resChannel <- true
		}

		client.idAwaiter.Lock()
		for _, request := range requests {
			client.idAwaiter.methods[request.ID] = method
		}
		client.idAwaiter.Unlock()

		go func() {
			time.Sleep(client.responseTimeout)
			for _, req := range requests {
				client.idAwaiter.RLock()
				_, ok := client.idAwaiter.methods[req.ID]
				client.idAwaiter.RUnlock()
				if ok {
					client.idAwaiter.Lock()
					delete(client.idAwaiter.methods, req.ID)
					client.idAwaiter.Unlock()
					err = newError("request timed out")
					resChannel <- true
				}
			}
		}()
		<-resChannel
		if err != nil {
			return err
		}
		err = json.Unmarshal(rpcResData, out)
		if err != nil {
			return newError(err.Error())
		}
	}
	return nil
}

func (client *rpcClient) CallBatch(requests RPCRequests) (RPCResponses, error) {
	rpcResponses := RPCResponses{}
	err := client.doBatchCall(&rpcResponses, requests)
	return rpcResponses, err
}

func (client *rpcClient) CallBatchFor(out interface{}, requests RPCRequests) error {
	err := client.doBatchCall(&out, requests)
	return err
}

func (client *rpcClient) ClientWS(w http.ResponseWriter, r *http.Request) error {
	c, err := client.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return newError(fmt.Sprintf("upgrade connection failed with err=%v", err))
	}
	client.Conn = c
	defer c.Close()

	for {
		_, message, err := c.ReadMessage()

		// normal closure
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			return nil
		}
		// abnormal closure
		if err != nil {
			return newError(fmt.Sprintf("read message failed with err=%v", err))
		}

		var method IDAwaiterMethod

		if ok, e := IsJsonArrBytes(message); e != nil {
			err = e
		} else if ok {
			rpcIDList := make([]rpcResponseID, 0, 1)
			err = json.Unmarshal(message, &rpcIDList)
			if err != nil {
				for _, item := range rpcIDList {
					client.idAwaiter.RLock()
					f, ok := client.idAwaiter.methods[item.ID]
					client.idAwaiter.RUnlock()
					if ok {
						method = f
						break
					}
				}
			}
		} else {
			var rpcID rpcResponseID
			err = json.Unmarshal(message, &rpcID)
			if err != nil {
				client.idAwaiter.RLock()
				f, ok := client.idAwaiter.methods[rpcID.ID]
				client.idAwaiter.RUnlock()
				if ok {
					method = f
				}
			}
		}
		if method != nil {
			method(message, newError(err))
		} else if err != nil {
			client.logger.Println(newError(err).Error())
		}
	}
}
