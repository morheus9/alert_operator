package main

import (
	"my-cdktf-go-project/stacks"

	"github.com/hashicorp/terraform-cdk-go/cdktf"
)

func main() {
	app := cdktf.NewApp(nil)
	stacks.NewDevStack(app, "dev-stack")
	app.Synth()
}
