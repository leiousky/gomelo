package errors

import (
	"fmt"
	"net/http"
)

type Code int

const (
	OK Code = 0

	BadRequest   Code = 400
	Unauthorized Code = 401
	Forbidden    Code = 403
	NotFound     Code = 404

	ServerError Code = 500
)

const (
	RouteNotFound    Code = 1001
	HandlerNotFound  Code = 1002
	SessionExpired    Code = 1003
	SessionNotFound  Code = 1004
	InvalidMessage   Code = 1005
	MessageTooBig    Code = 1006
	InvalidRoute     Code = 1007
	EncodeError      Code = 1008
	DecodeError      Code = 1009

	RPCError         Code = 2001
	RPCTimeout       Code = 2002
	RPCServerError   Code = 2003
	RPCClientError   Code = 2004
	RPCConnectError  Code = 2005
	RPCCallError     Code = 2006

	RegistryError    Code = 3001
	RegistryNotFound Code = 3002
	RegistryFull     Code = 3003

	PoolError        Code = 4001
	PoolExhausted    Code = 4002
	PoolClosed       Code = 4003

	NetworkError     Code = 5001
	ConnClosed       Code = 5002
	ConnRefused      Code = 5003
	ConnTimeout      Code = 5004
	SendError        Code = 5005
	RecvError        Code = 5006

	AuthError        Code = 6001
	AuthInvalidToken Code = 6002
	AuthExpired      Code = 6003
	AuthBanned       Code = 6004

	GameError        Code = 7001
	PlayerNotFound   Code = 7002
	PlayerOffline    Code = 7003
	SceneNotFound    Code = 7004
	BattleNotFound   Code = 7005
	TeamFull         Code = 7006
)

type GomeloError struct {
	Code    Code     `json:"code"`
	Message string   `json:"msg"`
	Detail  string   `json:"detail,omitempty"`
	Err     error    `json:"-"`
}

func (e *GomeloError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *GomeloError) Unwrap() error {
	return e.Err
}

func (e *GomeloError) WithDetail(detail string) *GomeloError {
	return &GomeloError{
		Code:    e.Code,
		Message: e.Message,
		Detail:  detail,
		Err:     e.Err,
	}
}

func (e *GomeloError) WithErr(err error) *GomeloError {
	return &GomeloError{
		Code:    e.Code,
		Message: e.Message,
		Detail:  e.Detail,
		Err:     err,
	}
}

func (e *GomeloError) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"code": e.Code,
		"msg":  e.Message,
	}
	if e.Detail != "" {
		m["detail"] = e.Detail
	}
	return m
}

func New(code Code, message string) *GomeloError {
	return &GomeloError{
		Code:    code,
		Message: message,
	}
}

func Newf(code Code, format string, args ...interface{}) *GomeloError {
	return New(code, fmt.Sprintf(format, args...))
}

func Wrap(code Code, err error, message string) *GomeloError {
	return &GomeloError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func Wrapf(code Code, err error, format string, args ...interface{}) *GomeloError {
	return Wrap(code, err, fmt.Sprintf(format, args...))
}

func (c Code) Error() string {
	return GetMessage(c)
}

func (c Code) WithMessage(msg string) *GomeloError {
	return &GomeloError{
		Code:    c,
		Message: msg,
	}
}

func (c Code) WithMessagef(format string, args ...interface{}) *GomeloError {
	return c.WithMessage(fmt.Sprintf(format, args...))
}

func (c Code) WithError(err error) *GomeloError {
	return &GomeloError{
		Code:    c,
		Message: GetMessage(c),
		Err:     err,
	}
}

func (c Code) WithDetail(detail string) *GomeloError {
	return &GomeloError{
		Code:    c,
		Message: GetMessage(c),
		Detail:  detail,
	}
}

func IsCode(err error, code Code) bool {
	if e, ok := err.(*GomeloError); ok {
		return e.Code == code
	}
	return false
}

func GetMessage(code Code) string {
	switch code {
	case OK:
		return "OK"
	case BadRequest:
		return "Bad Request"
	case Unauthorized:
		return "Unauthorized"
	case Forbidden:
		return "Forbidden"
	case NotFound:
		return "Not Found"
	case ServerError:
		return "Internal Server Error"
	case RouteNotFound:
		return "Route not found"
	case HandlerNotFound:
		return "Handler not found"
	case SessionExpired:
		return "Session expired"
	case SessionNotFound:
		return "Session not found"
	case InvalidMessage:
		return "Invalid message"
	case MessageTooBig:
		return "Message too big"
	case InvalidRoute:
		return "Invalid route"
	case EncodeError:
		return "Encode error"
	case DecodeError:
		return "Decode error"
	case RPCError:
		return "RPC error"
	case RPCTimeout:
		return "RPC timeout"
	case RPCServerError:
		return "RPC server error"
	case RPCClientError:
		return "RPC client error"
	case RPCConnectError:
		return "RPC connect error"
	case RPCCallError:
		return "RPC call error"
	case RegistryError:
		return "Registry error"
	case RegistryNotFound:
		return "Registry not found"
	case RegistryFull:
		return "Registry full"
	case PoolError:
		return "Pool error"
	case PoolExhausted:
		return "Pool exhausted"
	case PoolClosed:
		return "Pool closed"
	case NetworkError:
		return "Network error"
	case ConnClosed:
		return "Connection closed"
	case ConnRefused:
		return "Connection refused"
	case ConnTimeout:
		return "Connection timeout"
	case SendError:
		return "Send error"
	case RecvError:
		return "Receive error"
	case AuthError:
		return "Auth error"
	case AuthInvalidToken:
		return "Invalid token"
	case AuthExpired:
		return "Token expired"
	case AuthBanned:
		return "User banned"
	case GameError:
		return "Game error"
	case PlayerNotFound:
		return "Player not found"
	case PlayerOffline:
		return "Player offline"
	case SceneNotFound:
		return "Scene not found"
	case BattleNotFound:
		return "Battle not found"
	case TeamFull:
		return "Team full"
	default:
		return "Unknown error"
	}
}

func ToHTTPStatus(code Code) int {
	switch {
	case code >= 400 && code < 500:
		return http.StatusBadRequest + int(code-400)
	case code >= 500:
		return http.StatusInternalServerError
	case code >= 7000:
		return http.StatusInternalServerError
	default:
		return http.StatusOK
	}
}

type Response struct {
	Code    Code                `json:"code"`
	Msg     string              `json:"msg"`
	Data    interface{}         `json:"data,omitempty"`
	Detail  string              `json:"detail,omitempty"`
}

func NewResponse(err error) *Response {
	if err == nil {
		return &Response{Code: OK, Msg: "OK"}
	}
	if e, ok := err.(*GomeloError); ok {
		return &Response{
			Code:   e.Code,
			Msg:    e.Message,
			Detail: e.Detail,
		}
	}
	return &Response{Code: ServerError, Msg: err.Error()}
}

func NewResponseWithData(data interface{}) *Response {
	return &Response{
		Code: OK,
		Msg:  "OK",
		Data: data,
	}
}

func NewErrorResponse(code Code, msg string) *Response {
	return &Response{
		Code: code,
		Msg:  msg,
	}
}