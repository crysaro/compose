/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

import (
	"context"
	"strings"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
	containerType "github.com/docker/docker/api/types/container"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Restart(ctx context.Context, projectName string, options api.RestartOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.restart(ctx, strings.ToLower(projectName), options)
	})
}

func (s *composeService) restart(ctx context.Context, projectName string, options api.RestartOptions) error {
	containers, err := s.getContainers(ctx, projectName, oneOffExclude, true)
	if err != nil {
		return err
	}

	project := options.Project
	if project == nil {
		project, err = s.getProjectWithResources(ctx, containers, projectName)
		if err != nil {
			return err
		}
	}

	// ignore depends_on relations which are not impacted by restarting service
	for i, service := range project.Services {
		for name, r := range service.DependsOn {
			if !r.Restart {
				delete(service.DependsOn, name)
			}
		}
		project.Services[i] = service
	}

	if len(options.Services) == 0 {
		err = project.ForServices(options.Services)
		if err != nil {
			return err
		}
	}

	w := progress.ContextWriter(ctx)
	return InDependencyOrder(ctx, project, func(c context.Context, service string) error {
		eg, ctx := errgroup.WithContext(ctx)
		for _, container := range containers.filter(isService(service)) {
			container := container
			eg.Go(func() error {
				eventName := getContainerProgressName(container)
				w.Event(progress.RestartingEvent(eventName))
				timeout := utils.DurationSecondToInt(options.Timeout)
				err := s.apiClient().ContainerRestart(ctx, container.ID, containerType.StopOptions{Timeout: timeout})
				if err == nil {
					w.Event(progress.StartedEvent(eventName))
				}
				return err
			})
		}
		return eg.Wait()
	})
}
