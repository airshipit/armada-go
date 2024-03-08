/*
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package server

import (
	"fmt"
	policy "github.com/databus23/goslo.policy"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"net/http"
	"opendev.org/airship/armada-go/pkg/apply"
	auth2 "opendev.org/airship/armada-go/pkg/auth"
	"opendev.org/airship/armada-go/pkg/config"
	"opendev.org/airship/armada-go/pkg/log"
	"os"
	"strings"
)

// RunCommand phase run command
type RunCommand struct {
	Factory config.Factory
}

type JsonDataRequest struct {
	Href      string `json:"hrefs" binding:"required"`
	Overrides []any  `json:"overrides"`
}

func PolicyEnforcer(enforcer *policy.Enforcer, rule string) gin.HandlerFunc {
	return func(context *gin.Context) {
		ctx := policy.Context{
			Roles:  strings.Split(context.GetHeader("X-Roles"), ","),
			Logger: log.Printf,
		}
		if enforcer.Enforce(rule, ctx) {
			context.Next()
		} else {
			context.String(401, "oslo policy error")
		}
	}
}

func Apply(c *gin.Context) {
	if c.GetHeader("X-Identity-Status") == "Confirmed" {
		if c.ContentType() == "application/json" {
			targetManifest := c.Query("target_manifest")
			var dataReq JsonDataRequest
			if err := c.BindJSON(&dataReq); err != nil {
				c.String(500, "internal error", err.Error())
				return
			}

			runOpts := apply.RunCommand{Manifests: dataReq.Href, TargetManifest: targetManifest, Out: os.Stdout}
			if err := runOpts.RunE(); err != nil {
				c.String(500, "apply error", err.Error())
				return
			}

			c.JSON(200, gin.H{
				"message": gin.H{
					"install":   []any{},
					"upgrade":   []any{},
					"diff":      []any{},
					"purge":     []any{},
					"protected": []any{},
				},
			})
		} else {
			c.Status(500)
		}
	} else {
		c.Status(401)
	}
}

func Validate(c *gin.Context) {
	if c.GetHeader("X-Identity-Status") == "Confirmed" {
		c.JSON(200, gin.H{
			"kind":       "Status",
			"apiVersion": "v1.0",
			"metadata":   gin.H{},
			"reason":     "Validation",
			"details":    gin.H{"errorCount": 0, "messageList": []any{}},
			"status":     "Success",
			"message":    "Armada validations succeeded",
		})
		c.Status(200)
	} else {
		c.Status(401)
	}
}

func Releases(c *gin.Context) {
	if c.GetHeader("X-Identity-Status") == "Confirmed" {
		c.JSON(200, gin.H{
			"releases": gin.H{
				"ucp": []string{},
			},
		})
	} else {
		c.Status(401)
	}
}

func Health(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}

// RunE runs the phase
func (c *RunCommand) RunE() error {
	_, err := c.Factory()
	if err != nil {
		return err
	}

	log.Printf("armada-go server has been started")
	r := gin.Default()
	auth := auth2.New(viper.Sub("keystone_authtoken").GetString("auth_url"))

	buf, err := os.ReadFile("/etc/armada/policy.yaml")
	if err != nil {
		return err
	}

	var pol map[string]string
	err = yaml.Unmarshal(buf, &pol)
	if err != nil {
		return fmt.Errorf("in file %q: %w", "policy", err)
	}

	enf, err := policy.NewEnforcer(pol)
	if err != nil {
		return err
	}

	r.POST("/api/v1.0/apply", gin.Logger(), auth.Handler(r.Handler()), PolicyEnforcer(enf, "armada:create_endpoints"), Apply)
	r.POST("/api/v1.0/validatedesign", gin.Logger(), auth.Handler(r.Handler()), PolicyEnforcer(enf, "armada:validate_manifest"), Validate)
	r.GET("/api/v1.0/releases", gin.Logger(), auth.Handler(r.Handler()), PolicyEnforcer(enf, "armada:get_release"), Releases)
	r.GET("/api/v1.0/health", Health)
	return r.Run(":8000")
}
