package aws

import (
	"bytes"
	"encoding/base64"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/template"
)

var userdata = `#!/bin/bash
{{ .authorized }}
`

func UserDataBase64(user string) (*string, error) {
	model := map[string]interface{}{
		"authorized": provider.AuthorizedKeyScript(user),
	}
	w := &bytes.Buffer{}
	if err := template.OutputWithTemplate(userdata, w, model, nil); err != nil {
		panic(cmd.Error("failed to parse userdata template : %v", err))
	}
	userData := w.String()
	userData64 := base64.StdEncoding.EncodeToString([]byte(userData))
	return &userData64, nil
}
