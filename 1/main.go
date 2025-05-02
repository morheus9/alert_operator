package main

import (
	"github.com/hashicorp/terraform-cdk-go/cdktf"
	"github.com/morheus9/alert_operator/stacks"
)

func main() {
	app := cdktf.NewApp(nil)
	stacks.NewDevStack(app, "dev-stack")
	app.Synth()
}
