package main

import (
	"github.com/hashicorp/terraform-cdk-go/cdktf"

	"my-cdktf-go-project/stacks"
)

func main() {
	app := cdktf.NewApp(nil)
	stacks.NewDevStack(app, "dev-stack")
	app.Synth()
}
