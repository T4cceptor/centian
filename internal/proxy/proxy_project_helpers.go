package proxy

import (
	"github.com/T4cceptor/centian/internal/logging"
	"github.com/T4cceptor/centian/internal/taskverification"
)

func (p *CentianEndpoint) taskVerificationService() *taskverification.Service {
	if p == nil {
		return nil
	}
	if p.project != nil && p.project.TaskVerification != nil {
		return p.project.TaskVerification
	}
	if p.server != nil {
		return p.server.TaskVerification
	}
	return nil
}

func (p *CentianEndpoint) taskWorkingDir() string {
	service := p.taskVerificationService()
	if service == nil {
		return ""
	}
	return service.WorkingDir
}

func (p *CentianEndpoint) projectLogger() *logging.Logger {
	if p == nil {
		return nil
	}
	if p.project != nil && p.project.Logger != nil {
		return p.project.Logger
	}
	if p.server != nil {
		return p.server.Logger
	}
	return nil
}
