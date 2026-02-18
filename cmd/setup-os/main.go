package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

func main() {
	// Load configuration from environment
	keyPath := os.Getenv("OCI_PRIVATE_KEY_PATH")
	if keyPath == "" {
		keyPath = "./key.pem"
	}
	
	// Read private key content
	keyContent, err := os.ReadFile(keyPath)
	if err != nil {
		log.Fatalf("Failed to read private key file: %v", err)
	}
	
	configProvider := common.NewRawConfigurationProvider(
		os.Getenv("OCI_TENANCY_ID"),
		os.Getenv("OCI_USER_ID"),
		os.Getenv("OCI_REGION"),
		os.Getenv("OCI_FINGERPRINT"),
		string(keyContent),
		nil,
	)

	ctx := context.Background()

	// Create Object Storage client
	osClient, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(configProvider)
	if err != nil {
		log.Fatalf("Failed to create Object Storage client: %v", err)
	}

	// Get namespace
	namespaceReq := objectstorage.GetNamespaceRequest{}
	namespaceResp, err := osClient.GetNamespace(ctx, namespaceReq)
	if err != nil {
		log.Fatalf("Failed to get namespace: %v", err)
	}
	namespace := *namespaceResp.Value

	fmt.Printf("📦 Namespace: %s\n", namespace)

	// Create bucket
	bucketName := "postiz-media"
	createBucketReq := objectstorage.CreateBucketRequest{
		NamespaceName: &namespace,
		CreateBucketDetails: objectstorage.CreateBucketDetails{
			Name:          &bucketName,
			CompartmentId: common.String(os.Getenv("OCI_TENANCY_ID")),
			PublicAccessType: objectstorage.CreateBucketDetailsPublicAccessTypeNopublicaccess,
		},
	}

	_, err = osClient.CreateBucket(ctx, createBucketReq)
	if err != nil {
		// Check if bucket already exists
		if serviceErr, ok := common.IsServiceError(err); ok && serviceErr.GetHTTPStatusCode() == 409 {
			fmt.Printf("✅ Bucket '%s' already exists\n", bucketName)
		} else {
			log.Fatalf("Failed to create bucket: %v", err)
		}
	} else {
		fmt.Printf("✅ Bucket '%s' created successfully\n", bucketName)
	}

	// Create Identity client for Customer Secret Keys
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		log.Fatalf("Failed to create Identity client: %v", err)
	}

	// Create Customer Secret Key
	createSecretKeyReq := identity.CreateCustomerSecretKeyRequest{
		CreateCustomerSecretKeyDetails: identity.CreateCustomerSecretKeyDetails{
			DisplayName: common.String("postiz-s3-access"),
		},
		UserId: common.String(os.Getenv("OCI_USER_ID")),
	}

	secretKeyResp, err := identityClient.CreateCustomerSecretKey(ctx, createSecretKeyReq)
	if err != nil {
		log.Fatalf("Failed to create Customer Secret Key: %v", err)
	}

	fmt.Println("\n🔑 Customer Secret Key created successfully!")
	fmt.Printf("Access Key ID: %s\n", *secretKeyResp.CustomerSecretKey.Id)
	fmt.Printf("Secret Access Key: %s\n", *secretKeyResp.Key)
	fmt.Printf("\n⚠️  IMPORTANT: Save the Secret Access Key now - it won't be shown again!\n")

	// Generate S3 endpoint
	region := os.Getenv("OCI_REGION")
	s3Endpoint := fmt.Sprintf("https://%s.compat.objectstorage.%s.oraclecloud.com", namespace, region)
	publicURL := fmt.Sprintf("https://objectstorage.%s.oraclecloud.com/n/%s/b/%s/o/", region, namespace, bucketName)

	fmt.Printf("\n📝 Configuration for Postiz:\n")
	fmt.Printf("STORAGE_PROVIDER=s3\n")
	fmt.Printf("AWS_ACCESS_KEY_ID=%s\n", *secretKeyResp.CustomerSecretKey.Id)
	fmt.Printf("AWS_SECRET_ACCESS_KEY=%s\n", *secretKeyResp.Key)
	fmt.Printf("AWS_ENDPOINT=%s\n", s3Endpoint)
	fmt.Printf("AWS_BUCKET=%s\n", bucketName)
	fmt.Printf("AWS_REGION=%s\n", region)
	fmt.Printf("AWS_PUBLIC_URL=%s\n", publicURL)
	fmt.Printf("AWS_FORCE_PATH_STYLE=true\n")
}
