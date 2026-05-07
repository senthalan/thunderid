/*
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Package application exposes the public application service API.
package application

import (
	"context"

	inboundmodel "github.com/senthalan/thunder/backend/internal/inboundclient/model"
	"github.com/senthalan/thunder/backend/internal/system/error/serviceerror"
	"github.com/senthalan/thunder/backend/pkg/application/model"
)

// ApplicationServiceInterface defines the interface for the application service.
type ApplicationServiceInterface interface {
	CreateApplication(
		ctx context.Context, app *model.ApplicationDTO) (*model.ApplicationDTO, *serviceerror.ServiceError)
	ValidateApplication(ctx context.Context, app *model.ApplicationDTO) (
		*model.ApplicationProcessedDTO, *model.InboundAuthConfigDTO, *serviceerror.ServiceError)
	GetApplicationList(ctx context.Context) (*model.ApplicationListResponse, *serviceerror.ServiceError)
	GetOAuthApplication(
		ctx context.Context, clientID string) (*inboundmodel.OAuthClient, *serviceerror.ServiceError)
	GetApplication(ctx context.Context, appID string) (*model.Application, *serviceerror.ServiceError)
	UpdateApplication(
		ctx context.Context, appID string, app *model.ApplicationDTO) (
		*model.ApplicationDTO, *serviceerror.ServiceError)
	DeleteApplication(ctx context.Context, appID string) *serviceerror.ServiceError
}
