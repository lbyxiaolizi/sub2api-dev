import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import accountGroups from './accountGroups'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import audit from './audit'
import promptAudit from './promptAudit'
import promptRules from './promptRules'
import plugins from './plugins'

export default {
  ...overview,
  ...channels,
  ...accounts,
  ...accountGroups,
  ...resources,
  ...ops,
  ...settings,
  ...audit,
  ...promptAudit,
  ...promptRules,
  ...plugins,
}
