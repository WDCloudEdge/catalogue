package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/go-kit/kit/log"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"golang.org/x/net/context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const (
	ServiceName = "catalogue"
)

func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	// 创建 Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(ServiceName),
		)),
	)
	return tp, nil
}

func main() {

	var (
		port   = flag.String("port", "8080", "Port to bind HTTP listener") // TODO(pb): should be -addr, default ":80"
		images = flag.String("images", "./images/", "Image path")
		dsn    = flag.String("DSN", "catalogue_user:default_password@tcp(catalogue-db:3306)/socksdb", "Data Source Name: [username[:password]@][protocol[(address)]]/dbname")
	)
	flag.Parse()

	fmt.Fprintf(os.Stderr, "images: %q\n", *images)
	abs, err := filepath.Abs(*images)
	fmt.Fprintf(os.Stderr, "Abs(images): %q (%v)\n", abs, err)
	pwd, err := os.Getwd()
	fmt.Fprintf(os.Stderr, "Getwd: %q (%v)\n", pwd, err)
	files, _ := filepath.Glob(*images + "/*")
	fmt.Fprintf(os.Stderr, "ls: %q\n", files) // contains a list of all files in the current directory

	minioClient := InitMinioClient()
	bucketName := "sock"

	// Mechanical stuff.
	errc := make(chan error)
	ctx := context.Background()

	//Berr := minioClient.MakeBucket(bucketName, "cn-north-1")

	// Log domain.
	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = log.NewContext(logger).With("ts", log.DefaultTimestampUTC)
		logger = log.NewContext(logger).With("caller", log.DefaultCaller)
	}
	//for Berr != nil {
	//	exists, errBucketExists := minioClient.BucketExists(bucketName)
	//	if errBucketExists == nil && exists {
	//		logger.Log("bucket exist")
	//	} else {
	//		logger.Log(Berr.Error())
	//	}
	//	time.Sleep(5 * time.Second)
	//	Berr = minioClient.MakeBucket(bucketName, "cn-north-1")
	//}

	//filePath := "images"
	contentType := "multipart/form-data"
	for _, fp := range files {
		//fpath := filepath.Join(filePath, fp)
		fileInfo, _ := os.Stat(fp)
		_, err = minioClient.FPutObject(ctx, bucketName, fileInfo.Name(), fp, minio.PutObjectOptions{ContentType: contentType})
		if err != nil {
			logger.Log(err.Error())
		} else {
			logger.Log("put OK")
		}
	}

	tp, err := tracerProvider("http://172.26.146.180:14268/api/traces")
	if err == nil {
		logger.Log("jaeger OK")
	}
	go serveMetrics()

	// Find service local IP.
	otel.SetTracerProvider(tp)
	// Data domain.
	db, err := sqlx.Open("mysql", *dsn)
	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Check if DB connection can be made, only for logging purposes, should not fail/exit
	err = db.Ping()
	if err != nil {
		logger.Log("Error", "Unable to connect to Database", "DSN", dsn)
	}

	// Service domain.
	var service Service
	{
		service = NewCatalogueService(db, logger)
		service = LoggingMiddleware(logger)(service)
	}

	// Endpoint domain.
	endpoints := MakeEndpoints(service)

	// HTTP router
	router := MakeHTTPHandler(ctx, endpoints, *images, logger)

	// Handler

	// Create and launch the HTTP server.
	go func() {
		logger.Log("transport", "HTTP", "port", *port)
		errc <- http.ListenAndServe(":"+*port, router)
	}()

	// Capture interrupts.
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("%s", <-c)
	}()

	logger.Log("exit", <-errc)
}

func serveMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":9464", nil)
	if err != nil {
		fmt.Printf("error serving http: %v", err)
		return
	}
}

func InitMinioClient() *minio.Client {
	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = log.NewContext(logger).With("ts", log.DefaultTimestampUTC)
		logger = log.NewContext(logger).With("caller", log.DefaultCaller)
	}
	// 基本的配置信息
	endpoint := "myminio-hl.horsecoder-minio.svc.cluster.local:9000"
	accessKeyID := "KbbZXndsJvtcxYaTxxEn"
	secretAccessKey := "LmiuWWyebHKSyjqAG5BVlpfYL0uxyOSVYm279Cqk"
	//初始化一个minio客户端对象
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: true,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	})
	if err != nil {
		logger.Log(err.Error())
	} else {
		logger.Log("minio OK")
	}
	return minioClient
}
