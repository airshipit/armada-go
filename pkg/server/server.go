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
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"opendev.org/airship/armada-go/pkg/config"
	"opendev.org/airship/armada-go/pkg/log"
)

// RunCommand phase run command
type RunCommand struct {
	Factory config.Factory
}

func Apply(c *gin.Context) {
	if c.GetHeader("X-Identity-Status") == "Confirmed" {
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
				"ucp": []string{"clcp-ucp-armada"},
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
	r.Use(gin.Logger())
	auth := New(viper.Sub("keystone_authtoken").GetString("auth_url"))

	r.POST("/api/v1.0/apply", auth.Handler(r.Handler()), Apply)
	r.POST("/api/v1.0/validatedesign", auth.Handler(r.Handler()), Validate)
	r.GET("/api/v1.0/releases", auth.Handler(r.Handler()), Releases)
	r.GET("/api/v1.0/health", Health)
	return r.Run(":8000")
}
