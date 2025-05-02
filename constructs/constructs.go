package constructs

import (
	"github.com/hashicorp/terraform-cdk-go/cdktf"
	. "github.com/morheus9/alert_operator/.gen/github.com/yandex-cloud/yandex"
)

func NewYandexStorageBucket(scope cdktf.Construct, id string) cdktf.Resource {
	return storage.NewStorageBucket(scope, id, &storage.StorageBucketConfig{
		Bucket: cdktf.String("my-cdktf-test-bucket"),
	})
}
