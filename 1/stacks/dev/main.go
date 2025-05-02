package stacks

import (
	"github.com/hashicorp/terraform-cdk-go/cdktf"
	"github.com/morheus9/alert_operator/constructs"
)

type DevStack struct {
	cdktf.Stack
}

func NewDevStack(scope cdktf.Construct, id string) *DevStack {
	stack := &DevStack{}
	stack.Stack = cdktf.NewStack(scope, id, nil)

	// Создаем S3-совместимый бакет в Yandex Object Storage
	constructs.NewYandexStorageBucket(stack, "my-yandex-bucket")

	return stack
}
