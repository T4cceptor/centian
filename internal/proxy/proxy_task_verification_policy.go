package proxy

import "github.com/T4cceptor/centian/internal/config"

type gatewayTaskVerificationPolicy struct {
	requirement string
}

func (p gatewayTaskVerificationPolicy) exposesTaskTools() bool {
	return p.requirement != config.VerificationRequirementOff
}

func (p gatewayTaskVerificationPolicy) requiresRegistration() bool {
	return p.requirement == config.VerificationRequirementRequired
}

func (p gatewayTaskVerificationPolicy) enforcesActiveTaskGovernance() bool {
	return p.requirement != config.VerificationRequirementOff
}

func (p *CentianEndpoint) taskVerificationPolicy() gatewayTaskVerificationPolicy {
	if p == nil {
		return gatewayTaskVerificationPolicy{requirement: config.VerificationRequirementOff}
	}

	if !p.projectTaskVerificationEnabled() {
		return gatewayTaskVerificationPolicy{requirement: config.VerificationRequirementOff}
	}

	requirement := config.VerificationRequirementRequired
	if p.config != nil && p.config.NormalizedVerificationRequirement() != "" {
		requirement = p.config.NormalizedVerificationRequirement()
	}

	return gatewayTaskVerificationPolicy{requirement: requirement}
}

func (p *CentianEndpoint) projectTaskVerificationEnabled() bool {
	if p == nil {
		return false
	}
	if p.project != nil && p.project.Config != nil {
		return p.project.Config.TaskVerificationEnabled()
	}
	return p.server != nil && p.server.Config != nil && p.server.Config.Proxy != nil &&
		p.server.Config.Proxy.TaskVerificationEnabled()
}
