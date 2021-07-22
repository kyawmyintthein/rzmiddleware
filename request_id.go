package rzmiddleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	multiint "github.com/mercari/go-grpc-interceptor/multiinterceptor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type RequestIDKey struct{}

const (
	RequestIDHeader = "X-Request-Id"
)

var (
	prefix string
	reqid  uint64
)

func init() {
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}
	var buf [12]byte
	var b64 string
	for len(b64) < 10 {
		rand.Read(buf[:])
		b64 = base64.StdEncoding.EncodeToString(buf[:])
		b64 = strings.NewReplacer("+", "", "/", "").Replace(b64)
	}

	prefix = fmt.Sprintf("%s/%s", hostname, b64[0:10])
}

func RequestID(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
			r.Header.Set(RequestIDHeader, requestID)
		}
		ctx = context.WithValue(ctx, RequestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(fn)
}

func newRequestID() string {
	myid := atomic.AddUint64(&reqid, 1)
	return fmt.Sprintf("%s-%06d", prefix, myid)
}

func RequestIDUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.Pairs(
				RequestIDHeader, newRequestID(),
			)
		} else {
			reqIDVals := md.Get(RequestIDHeader)
			if len(reqIDVals) == 0 {
				md.Set(RequestIDHeader, newRequestID())
			}
		}

		newCtx := metadata.NewIncomingContext(ctx, md)
		return handler(newCtx, req)
	}
}

func RequestIDStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		ctx := stream.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.Pairs(
				RequestIDHeader, newRequestID(),
			)
		} else {
			reqIDVals := md.Get(RequestIDHeader)
			if len(reqIDVals) == 0 {
				md.Set(RequestIDHeader, newRequestID())
			}
		}
		newCtx := metadata.NewIncomingContext(ctx, md)
		stream = multiint.NewServerStreamWithContext(stream, newCtx)
		return handler(srv, stream)
	}
}

func GetReqID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(RequestIDKey{}).(string); ok {
		return reqID
	}
	return ""
}
