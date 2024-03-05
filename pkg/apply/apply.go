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

package apply

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"opendev.org/airship/armada-go/pkg/auth"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"opendev.org/airship/armada-go/pkg/config"
	armadav1 "opendev.org/airship/armada-operator/api/v1"
	armadawait "opendev.org/airship/armada-operator/pkg/waitutil"
)

// RunCommand phase run command
type RunCommand struct {
	Factory        config.Factory
	Manifests      string
	TargetManifest string
	Out            io.Writer

	airManifest *AirshipManifest
	airGroups   map[string]*AirshipChartGroup
	airCharts   map[string]*AirshipChart
}

type AirshipDocument struct {
	Schema   string          `json:"schema,omitempty"`
	Metadata AirshipMetadata `json:"metadata,omitempty"`
}

type AirshipMetadata struct {
	Name string `json:"name,omitempty"`
}

type AirshipManifest struct {
	AirshipDocument
	AirshipManifestSpec `json:"data,omitempty"`
}

type AirshipManifestSpec struct {
	ChartGroups   []string `json:"chart_groups,omitempty"`
	ReleasePrefix string   `json:"release_prefix,omitempty"`
}

type AirshipChartGroup struct {
	AirshipDocument
	AirshipChartGroupSpec `json:"data,omitempty"`
}

type AirshipChartGroupSpec struct {
	ChartGroup  []string `json:"chart_group,omitempty"`
	Description string   `json:"description,omitempty"`
	Sequenced   bool     `json:"sequenced,omitempty"`
}

type AirshipChart struct {
	AirshipDocument
	armadav1.ArmadaChartSpec `json:"data,omitempty"`
}

// RunE runs the phase
func (c *RunCommand) RunE() error {
	klog.InitFlags(nil)
	klog.SetOutput(c.Out)
	if err := flag.Set("v", "5"); err != nil {
		return err
	}
	klog.V(2).Infof("armada-go apply, manifests path %s", c.Manifests)

	if err := c.ParseManifests(); err != nil {
		return err
	}

	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.V(2).Infoln("Unable to load in-cluster kubeconfig, reason: ", err)
		k8sConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			return err
		}
	}

	if err := c.VerifyNamespaces(k8sConfig); err != nil {
		return err
	}

	dc := dynamic.NewForConfigOrDie(k8sConfig)
	resClient := dc.Resource(schema.GroupVersionResource{
		Group:    armadav1.ArmadaChartGroup,
		Version:  armadav1.ArmadaChartVersion,
		Resource: armadav1.ArmadaChartPlural,
	})

	if err := c.CheckCRD(k8sConfig); err != nil {
		return err
	}

	for _, cgName := range c.airManifest.ChartGroups {
		cg := c.airGroups[cgName]
		klog.V(5).Infof("processing chart group %s, sequenced %s", cgName, cg.Sequenced)
		if !cg.Sequenced {
			eg := errgroup.Group{}
			for _, cName := range cg.ChartGroup {
				klog.V(5).Infof("adding 1 chart to wg %s", cName)
				chp := c.airCharts[cName]
				chpc := c.ConvertChart(chp)
				eg.Go(func() error {
					return c.InstallChart(chpc, resClient, k8sConfig)
				})
			}
			if err := eg.Wait(); err != nil {
				return err
			}
		} else {
			for _, cName := range cg.ChartGroup {
				klog.V(5).Infof("sequential chart install %s", cName)
				if err = c.InstallChart(c.ConvertChart(c.airCharts[cName]), resClient, k8sConfig); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *RunCommand) InstallChart(
	chart *armadav1.ArmadaChart,
	resClient dynamic.NamespaceableResourceInterface,
	restConfig *rest.Config) error {

	klog.V(5).Infof("installing chart %s %s %s", chart.GetName(), chart.Name, chart.Namespace)
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(chart)
	if err != nil {
		return err
	}

	if oldObj, err := resClient.Namespace(chart.Namespace).Get(
		context.Background(), chart.GetName(), metav1.GetOptions{}); err != nil {
		klog.V(5).Infof("unable to get chart %s: %s, creating", chart.Name, err.Error())
		if _, err = resClient.Namespace(chart.Namespace).Create(
			context.Background(), &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{}); err != nil {
			return err
		}
		klog.V(5).Infof("chart has been successfully created %s", chart.Name)
	} else {
		uObj := &unstructured.Unstructured{Object: obj}
		uObj.SetResourceVersion(oldObj.GetResourceVersion())
		klog.V(5).Infof("chart %s was found, updating", chart.Name)
		if _, err = resClient.Namespace(chart.Namespace).Update(
			context.Background(), uObj, metav1.UpdateOptions{}); err != nil {
			klog.V(5).Infof("resource update error: %s", err.Error())
			if strings.Contains(err.Error(), "the object has been modified") {
				klog.V(5).Infof("resource expired, retrying %s", err.Error())
				return c.InstallChart(chart, resClient, restConfig)
			}
			return err
		}
		klog.V(5).Infof("chart has been successfully updated %s", chart.Name)
	}

	wOpts := armadawait.WaitOptions{
		RestConfig: restConfig,
		Namespace:  chart.Namespace,
		LabelSelector: fmt.Sprintf("%s=%s", armadav1.ArmadaChartLabel,
			fmt.Sprintf("%s-%s", c.airManifest.ReleasePrefix, chart.Spec.Release)),
		ResourceType: "armadacharts",
		Timeout:      time.Second * time.Duration(chart.Spec.Wait.Timeout),
		Logger:       klog.FromContext(context.Background()),
	}

	err = wOpts.Wait(context.Background())
	klog.V(5).Infof("finished with chart %s", chart.GetName())
	return err
}

func (c *RunCommand) ConvertChart(chart *AirshipChart) *armadav1.ArmadaChart {
	return &armadav1.ArmadaChart{
		TypeMeta: metav1.TypeMeta{
			Kind:       armadav1.ArmadaChartKind,
			APIVersion: armadav1.ArmadaChartAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", c.airManifest.ReleasePrefix, chart.Release),
			Namespace: chart.Namespace,
			Labels: map[string]string{
				armadav1.ArmadaChartLabel: fmt.Sprintf("%s-%s", c.airManifest.ReleasePrefix, chart.Release),
			},
		},
		Spec: chart.ArmadaChartSpec,
	}
}

func (c *RunCommand) CheckCRD(restConfig *rest.Config) error {
	crdClient := apiextension.NewForConfigOrDie(restConfig)
	if _, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), "armadacharts.armada.airshipit.org", metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(5).Infof("armadacharts CRD not found, creating: %s", err.Error())
			objToapp, err := c.ReadCRD()
			if err != nil {
				return err
			}
			_, err = crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.Background(), objToapp, metav1.CreateOptions{})
			if err != nil {
				klog.V(5).Infof("error while creating crd %t", err)
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func (c *RunCommand) ReadCRD() (*apiextv1.CustomResourceDefinition, error) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = apiextv1.AddToScheme(sch)

	decode := serializer.NewCodecFactory(sch).UniversalDeserializer().Decode
	data, err := os.ReadFile("crd.yaml")
	if err != nil {
		return nil, err
	}
	obj, _, err := decode(data, nil, nil)
	if err != nil {
		return nil, err
	}

	crdTo := obj.(*apiextv1.CustomResourceDefinition)
	return crdTo, nil
}

func (c *RunCommand) VerifyNamespaces(rsc *rest.Config) error {
	cs := kubernetes.NewForConfigOrDie(rsc)

	namespaces := make(map[string]bool)
	for _, cgname := range c.airManifest.ChartGroups {
		cg := c.airGroups[cgname]
		for _, chrt := range cg.ChartGroup {
			ns := c.airCharts[chrt].Namespace
			if _, ok := namespaces[ns]; !ok {
				namespaces[ns] = true
			}
		}
	}
	for k, _ := range namespaces {
		klog.V(5).Infof("processing namespace %s", k)
		if _, err := cs.CoreV1().Namespaces().Get(context.Background(), k, metav1.GetOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				klog.V(5).Infof("namespace %s not found, creating", k)
				if _, err = cs.CoreV1().Namespaces().Create(context.Background(), &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: k}}, metav1.CreateOptions{}); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}
	klog.V(5).Infof("all namespaces validated successfully")
	return nil
}

func (c *RunCommand) ValidateManifests() error {
	if c.airManifest == nil {
		return errors.New("no or multiple armada manifest found")
	}

	for _, cgname := range c.airManifest.ChartGroups {
		if cg, ok := c.airGroups[cgname]; ok {
			for _, cName := range cg.ChartGroup {
				if chrt, ok := c.airCharts[cName]; ok {
					if chrt.Release == "" || chrt.Namespace == "" {
						return errors.New(fmt.Sprintf("chart document with name %s found does not have release or ns", cName))
					}
				} else {
					return errors.New(fmt.Sprintf("no chart document with name %s found", cName))
				}
			}
		} else {
			return errors.New(fmt.Sprintf("no group document with name %s found", cgname))
		}
	}
	klog.V(5).Infof("all airship manifests validated successfully")
	return nil
}

func (c *RunCommand) ParseManifests() error {
	klog.V(5).Infof("parsing manifests started, path: %s", c.Manifests)

	var f io.ReadCloser
	u, err := url.Parse(c.Manifests)
	if err != nil {
		return err
	}
	if u.Scheme == "" {
		f, err = os.Open(c.Manifests)
		if err != nil {
			return err
		}
	} else if u.Scheme == "deckhand+http" {
		reg, err := regexp.Compile("^[^+]+\\+")
		if err != nil {
			return err
		}
		deckhandUrl := reg.ReplaceAllString(c.Manifests, "")
		req, err := http.NewRequest("GET", deckhandUrl, nil)
		if err != nil {
			return err
		}
		token, err := auth.Authenticate()
		if err != nil {
			return err
		}
		req.Header.Set("X-Auth-Token", token)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		f = resp.Body
	}
	defer f.Close()

	c.airCharts = map[string]*AirshipChart{}
	c.airGroups = map[string]*AirshipChartGroup{}
	multidocReader := utilyaml.NewYAMLReader(bufio.NewReader(f))
	for {
		buf, err := multidocReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		var typeMeta AirshipDocument
		if err := yaml.Unmarshal(buf, &typeMeta); err != nil {
			klog.V(2).Infof("unmarshalling error %s, continuing...", err.Error())
			continue
		}

		if typeMeta.Schema == "armada/Manifest/v1" {
			if (c.TargetManifest != "" && typeMeta.Metadata.Name == c.TargetManifest) ||
				(c.TargetManifest == "" && c.airManifest == nil) {
				var airManifest AirshipManifest
				if err := yaml.Unmarshal(buf, &airManifest); err != nil {
					return err
				}
				klog.V(2).Infof("found airship manifest %s", airManifest.Metadata.Name)
				c.airManifest = &airManifest
			}
		}
		if typeMeta.Schema == "armada/ChartGroup/v1" {
			var cg AirshipChartGroup
			if err := yaml.Unmarshal(buf, &cg); err != nil {
				return err
			}
			c.airGroups[typeMeta.Metadata.Name] = &cg
		}

		if typeMeta.Schema == "armada/Chart/v1" {
			var chrt AirshipChart
			if err := yaml.Unmarshal(buf, &chrt); err != nil {
				return err
			}
			c.airCharts[typeMeta.Metadata.Name] = &chrt
		}
	}

	return c.ValidateManifests()
}
