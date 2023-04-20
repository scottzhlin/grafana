package navtreeimpl

import (
	ac "github.com/grafana/grafana/pkg/services/accesscontrol"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/correlations"
	"github.com/grafana/grafana/pkg/services/datasources"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/grafana/grafana/pkg/services/navtree"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/services/pluginsintegration/pluginaccesscontrol"
	"github.com/grafana/grafana/pkg/services/serviceaccounts"
)

func (s *ServiceImpl) getOrgAdminNode(c *contextmodel.ReqContext) (*navtree.NavLink, error) {
	var configNodes []*navtree.NavLink

	hasAccess := ac.HasAccess(s.accessControl, c)
	if hasAccess(ac.ReqOrgAdmin, datasources.ConfigurationPageAccess) {
		configNodes = append(configNodes, &navtree.NavLink{
			Text:     "Data sources",
			Icon:     "database",
			SubTitle: "Add and configure data sources",
			Id:       "datasources",
			Url:      s.cfg.AppSubURL + "/datasources",
		})
	}

	if s.features.IsEnabled(featuremgmt.FlagCorrelations) && hasAccess(ac.ReqOrgAdmin, correlations.ConfigurationPageAccess) {
		configNodes = append(configNodes, &navtree.NavLink{
			Text:     "Correlations",
			Icon:     "gf-glue",
			SubTitle: "Add and configure correlations",
			Id:       "correlations",
			Url:      s.cfg.AppSubURL + "/datasources/correlations",
		})
	}

	if hasAccess(s.ReqCanAdminTeams, ac.TeamsAccessEvaluator) {
		configNodes = append(configNodes, &navtree.NavLink{
			Text:     "Teams",
			Id:       "teams",
			SubTitle: "Groups of users that have common dashboard and permission needs",
			Icon:     "users-alt",
			Url:      s.cfg.AppSubURL + "/org/teams",
		})
	}

	// FIXME: while we don't have a permissions for listing plugins the legacy check has to stay as a default
	if pluginaccesscontrol.ReqCanAdminPlugins(s.cfg)(c) || hasAccess(pluginaccesscontrol.ReqCanAdminPlugins(s.cfg), pluginaccesscontrol.AdminAccessEvaluator) {
		configNodes = append(configNodes, &navtree.NavLink{
			Text:     "Plugins",
			Id:       "plugins",
			SubTitle: "Extend the Grafana experience with plugins",
			Icon:     "plug",
			Url:      s.cfg.AppSubURL + "/plugins",
		})
	}

	if hasAccess(ac.ReqOrgAdmin, ac.OrgPreferencesAccessEvaluator) {
		configNodes = append(configNodes, &navtree.NavLink{
			Text:     "Preferences",
			Id:       "org-settings",
			SubTitle: "Manage preferences across an organization",
			Icon:     "sliders-v-alt",
			Url:      s.cfg.AppSubURL + "/org",
		})
	}

	disabled, err := s.apiKeyService.IsDisabled(c.Req.Context(), c.OrgID)
	if err != nil {
		return nil, err
	}
	if hasAccess(ac.ReqOrgAdmin, ac.ApiKeyAccessEvaluator) && !disabled {
		configNodes = append(configNodes, &navtree.NavLink{
			Text:     "API keys",
			Id:       "apikeys",
			SubTitle: "Manage and create API keys that are used to interact with Grafana HTTP APIs",
			Icon:     "key-skeleton-alt",
			Url:      s.cfg.AppSubURL + "/org/apikeys",
		})
	}

	if enableServiceAccount(s, c) {
		configNodes = append(configNodes, &navtree.NavLink{
			Text:     "Service accounts",
			Id:       "serviceaccounts",
			SubTitle: "Use service accounts to run automated workloads in Grafana",
			Icon:     "gf-service-account",
			Url:      s.cfg.AppSubURL + "/org/serviceaccounts",
		})
	}

	configNode := &navtree.NavLink{
		Id:         navtree.NavIDCfg,
		Text:       "Configuration",
		SubTitle:   "Organization: " + c.OrgName,
		Icon:       "cog",
		SortWeight: navtree.WeightConfig,
		Children:   configNodes,
	}

	return configNode, nil
}

func (s *ServiceImpl) getServerAdminNode(c *contextmodel.ReqContext) *navtree.NavLink {
	hasAccess := ac.HasAccess(s.accessControl, c)
	hasGlobalAccess := ac.HasGlobalAccess(s.accessControl, s.accesscontrolService, c)
	orgsAccessEvaluator := ac.EvalPermission(ac.ActionOrgsRead)
	adminNavLinks := []*navtree.NavLink{}

	if hasAccess(ac.ReqSignedIn, ac.EvalAny(ac.EvalPermission(ac.ActionOrgUsersRead), ac.EvalPermission(ac.ActionUsersRead, ac.ScopeGlobalUsersAll))) {
		adminNavLinks = append(adminNavLinks, &navtree.NavLink{
			Text: "Users", SubTitle: "Manage users in Grafana", Id: "global-users", Url: s.cfg.AppSubURL + "/admin/users", Icon: "user",
		})
	}

	authConfigUIAvailable := s.license.FeatureEnabled("saml") && s.features.IsEnabled(featuremgmt.FlagAuthenticationConfigUI)
	if authConfigUIAvailable && hasAccess(ac.ReqGrafanaAdmin, evalAuthenticationSettings()) {
		adminNavLinks = append(adminNavLinks, &navtree.NavLink{
			Text:     "Authentication",
			Id:       "authentication",
			SubTitle: "Manage your auth settings and configure single sign-on",
			Icon:     "signin",
			Url:      s.cfg.AppSubURL + "/admin/authentication",
		})
	}

	if hasGlobalAccess(ac.ReqGrafanaAdmin, orgsAccessEvaluator) {
		adminNavLinks = append(adminNavLinks, &navtree.NavLink{
			Text: "Organizations", SubTitle: "Isolated instances of Grafana running on the same server", Id: "global-orgs", Url: s.cfg.AppSubURL + "/admin/orgs", Icon: "building",
		})
	}

	if hasAccess(ac.ReqGrafanaAdmin, ac.EvalPermission(ac.ActionSettingsRead)) {
		adminNavLinks = append(adminNavLinks, &navtree.NavLink{
			Text: "Settings", SubTitle: "View the settings defined in your Grafana config", Id: "server-settings", Url: s.cfg.AppSubURL + "/admin/settings", Icon: "sliders-v-alt",
		})
	}

	if hasAccess(ac.ReqGrafanaAdmin, ac.EvalPermission(ac.ActionSettingsRead)) && s.features.IsEnabled(featuremgmt.FlagStorage) {
		storage := &navtree.NavLink{
			Text:     "Storage",
			Id:       "storage",
			SubTitle: "Manage file storage",
			Icon:     "cube",
			Url:      s.cfg.AppSubURL + "/admin/storage",
		}
		adminNavLinks = append(adminNavLinks, storage)
	}

	if s.cfg.LDAPAuthEnabled && hasAccess(ac.ReqGrafanaAdmin, ac.EvalPermission(ac.ActionLDAPStatusRead)) {
		adminNavLinks = append(adminNavLinks, &navtree.NavLink{
			Text: "LDAP", Id: "ldap", Url: s.cfg.AppSubURL + "/admin/ldap", Icon: "book",
		})
	}

	adminNode := &navtree.NavLink{
		Text:       "Server admin",
		Id:         navtree.NavIDAdmin,
		Icon:       "shield",
		SortWeight: navtree.WeightAdmin,
		Children:   adminNavLinks,
	}

	if len(adminNavLinks) > 0 {
		adminNode.Url = adminNavLinks[0].Url
	}

	return adminNode
}

func (s *ServiceImpl) ReqCanAdminTeams(c *contextmodel.ReqContext) bool {
	return c.OrgRole == org.RoleAdmin || (s.cfg.EditorsCanAdmin && c.OrgRole == org.RoleEditor)
}

func enableServiceAccount(s *ServiceImpl, c *contextmodel.ReqContext) bool {
	hasAccess := ac.HasAccess(s.accessControl, c)
	return hasAccess(ac.ReqOrgAdmin, serviceaccounts.AccessEvaluator)
}

func evalAuthenticationSettings() ac.Evaluator {
	return ac.EvalAll(
		ac.EvalPermission(ac.ActionSettingsWrite, ac.ScopeSettingsAuth),
		ac.EvalPermission(ac.ActionSettingsWrite, ac.ScopeSettingsSAML),
		ac.EvalPermission(ac.ActionSettingsRead, ac.ScopeSettingsAuth),
		ac.EvalPermission(ac.ActionSettingsRead, ac.ScopeSettingsSAML),
	)
}
