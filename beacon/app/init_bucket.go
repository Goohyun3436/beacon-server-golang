package main

import (
	"context"
	"fmt"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/domain"
)

func main() {
	url := "http://influxdb2:8086"
	token := "2bq9r-S3qb3e-ter4-w2kid"
	orgName := "beacon"
	bucketName := "beacon"

	client := influxdb2.NewClient(url, token)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check health
	health, err := client.Health(ctx)
	if err != nil || health.Status != "pass" {
		fmt.Println("❌ InfluxDB Unhealthy:", err)
		os.Exit(1)
	}

	// Get org ID from org name
	orgAPI := client.OrganizationsAPI()
	org, err := orgAPI.FindOrganizationByName(ctx, orgName)
	if err != nil || org == nil {
		fmt.Printf("❌ Failed to find org '%s': %v\n", orgName, err)
		os.Exit(1)
	}

	// Check if bucket exists
	bucketsAPI := client.BucketsAPI()
	buckets, err := bucketsAPI.FindBucketsByOrgName(ctx, orgName)
	if err != nil {
		fmt.Println("❌ Failed to retrieve buckets:", err)
		os.Exit(1)
	}
	for _, b := range *buckets {
		if b.Name == bucketName {
			fmt.Printf("[INIT] Bucket '%s' already exists\n", bucketName)
			return
		}
	}

	// Create new bucket
	ruleType := domain.RetentionRuleTypeExpire
	newBucket := &domain.Bucket{
		OrgID: org.Id,
		Name:  bucketName,
		RetentionRules: []domain.RetentionRule{
			{
				EverySeconds: int64(2 * 24 * 60 * 60),
				Type:         &ruleType,
			},
		},
	}

	_, err = bucketsAPI.CreateBucket(ctx, newBucket)
	if err != nil {
		fmt.Printf("❌ Failed to create bucket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[INIT] Bucket '%s' created successfully\n", bucketName)
}