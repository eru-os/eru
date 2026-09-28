package tools_factory

import (
	tools "github.com/eru-os/eru/eru-ai/tools"
	_ "github.com/eru-os/eru/eru-ai/tools/analytics"
	_ "github.com/eru-os/eru/eru-ai/tools/ecomm"
	_ "github.com/eru-os/eru/eru-ai/tools/emails"
	_ "github.com/eru-os/eru/eru-ai/tools/messengers"
	_ "github.com/eru-os/eru/eru-ai/tools/repositories"
	_ "github.com/eru-os/eru/eru-ai/tools/saas"
	_ "github.com/eru-os/eru/eru-ai/tools/utility"
	_ "github.com/eru-os/eru/eru-ai/tools/vectors"
)

func GetTool(toolType string) tools.Tooling {
	return tools.NewTool(toolType)
}
