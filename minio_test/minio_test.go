package minio_test

import (
	"crypto/tls"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"strings"
	"testing"
)

func TestMinio(t *testing.T) {
	endpoint := "minio-svc.:31190"
	accessKeyID := "KbbZXndsJvtcxYaTxxEn"
	secretAccessKey := "LmiuWWyebHKSyjqAG5BVlpfYL0uxyOSVYm279Cqk"
	// 初始化一个minio客户端对象
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: true,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	})
	if err != nil {
		log.Print(err.Error())
	} else {
		log.Print(minioClient.EndpointURL())
	}
	_, err = minioClient.BucketExists(context.Background(), "sock")

	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Successfully created mybucket.")

	str := "path/to/your/filename.txt"

	// 找到最后一个"/"字符的索引
	lastIndex := strings.LastIndex(str, "/")

	if lastIndex != -1 {
		// 使用切片操作获取从最后一个"/"字符到字符串末尾的子字符串
		subStr := str[lastIndex+1:]
		log.Print(subStr)
	}
}
