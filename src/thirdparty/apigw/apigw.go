/*
 * Tencent is pleased to support the open source community by making
 * 蓝鲸智云 - 配置平台 (BlueKing - Configuration System) available.
 * Copyright (C) 2017 THL A29 Limited,
 * a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 * We undertake not to change the open source license (MIT license) applicable
 * to the current version of the project delivered to anyone in the future.
 */

package apigw

import (
	"sync"

	"configcenter/src/apimachinery/rest"
	"configcenter/src/common/blog"
	"configcenter/src/thirdparty/apigw/apigwutil"
	"configcenter/src/thirdparty/apigw/gse"
	"configcenter/src/thirdparty/apigw/notice"

	"github.com/prometheus/client_golang/prometheus"
)

type ApiGWI interface {
	Notice() notice.NoticeClientInterface
	Gse() gse.GseClientInterface
}

var apiGWCli *apiGW

// InitApiGW init api gateway client
func InitApiGW(reg prometheus.Registerer) error {
	if apiGWCli != nil {
		return nil
	}

	config, err := apigwutil.ParseApiGWConfig("apiGW")
	if err != nil {
		blog.Errorf("get api gateway config error, err: %v", err)
		return err
	}

	cliConf, err := apigwutil.NewConfig(config, reg)
	if err != nil {
		blog.Errorf("new api gateway failed, err: %v", err)
		return err
	}
	apiGWCli = &apiGW{config: cliConf}

	return nil
}

// Client get api gatewat client
func Client() ApiGWI {
	return apiGWCli
}

type apiGW struct {
	sync.RWMutex
	config *apigwutil.CliConf
	notice notice.NoticeClientInterface
	gse    gse.GseClientInterface
}

// Notice get notice api gateway client
func (a *apiGW) Notice() notice.NoticeClientInterface {
	a.RLock()
	cli := a.notice
	a.RUnlock()
	if cli == nil {
		a.Lock()
		capability := a.config.Capability
		capability.Discover = &apigwutil.ApiGWDiscovery{
			Servers: apigwutil.ReplaceApiName(a.config.Address, apigwutil.NoticeName),
		}
		a.notice = notice.NewNoticeApiGWClient(a.config.Auth, rest.NewRESTClient(&capability, "/"))
		cli = a.notice
		a.Unlock()
	}

	return cli
}

// Gse get gse api gateway client
func (a *apiGW) Gse() gse.GseClientInterface {
	a.RLock()
	cli := a.gse
	a.RUnlock()
	if cli == nil {
		a.Lock()
		capability := a.config.Capability
		capability.Discover = &apigwutil.ApiGWDiscovery{
			Servers: apigwutil.ReplaceApiName(a.config.Address, apigwutil.GseName),
		}
		a.gse = gse.NewGseApiGWClient(a.config.Auth, rest.NewRESTClient(&capability, "/"))
		cli = a.gse
		a.Unlock()
	}

	return cli
}
