import { css } from '@emotion/css';
import React, { useMemo } from 'react';

import { GrafanaTheme2, SelectableValue } from '@grafana/data';
import { InlineField, Select, useStyles2 } from '@grafana/ui';
import { AlertmanagerAction, getAlertmanagerAbilities } from 'app/features/alerting/unified/hooks/useAbilities';

import { useAlertmanager } from '../state/AlertmanagerContext';
import { AlertManagerDataSource, GRAFANA_RULES_SOURCE_NAME } from '../utils/datasource';

interface Props {
  disabled?: boolean;
  /**
   * If true, only show alertmanagers that are receiving alerts from Grafana
   */
  showOnlyReceivingGrafanaAlerts?: boolean;
  /**
   * If provided, only show alertmanagers where the user has permissions to perform all actions
   */
  actions?: AlertmanagerAction[];
}

function getAlertManagerLabel(alertManager: AlertManagerDataSource) {
  return alertManager.name === GRAFANA_RULES_SOURCE_NAME ? 'Grafana' : alertManager.name.slice(0, 37);
}

export const AlertManagerPicker = ({ disabled = false, showOnlyReceivingGrafanaAlerts, actions }: Props) => {
  const styles = useStyles2(getStyles);
  const { selectedAlertmanager, availableAlertManagers, setSelectedAlertmanager } = useAlertmanager();

  const options: Array<SelectableValue<string>> = useMemo(() => {
    return availableAlertManagers
      .filter(({ name, handleGrafanaManagedAlerts, hasConfigurationAPI }) => {
        const isReceivingGrafanaAlerts = name === GRAFANA_RULES_SOURCE_NAME || handleGrafanaManagedAlerts;
        /** Should we filter out this alertmanager due to not receiving grafana alerts? */
        const receivingFilter = showOnlyReceivingGrafanaAlerts ? isReceivingGrafanaAlerts : true;

        /** Should we filter out this alertmanager due to the user not having the correct permissions? */
        const actionsFilter = (() => {
          if (!actions || actions.length === 0) {
            return true;
          }

          const abilities = getAlertmanagerAbilities({
            alertmanagerName: name,
            hasConfigurationAPI: Boolean(hasConfigurationAPI),
            isGrafanaFlavoredAlertmanager: name === GRAFANA_RULES_SOURCE_NAME,
          });

          /* Make sure that every action we asked about is supported and can be performed by the user */
          return actions.every((action) => abilities[action].every(Boolean));
        })();

        return receivingFilter && actionsFilter;
      })
      .map((ds) => ({
        label: getAlertManagerLabel(ds),
        value: ds.name,
        imgUrl: ds.imgUrl,
        meta: ds.meta,
      }));
  }, [actions, availableAlertManagers, showOnlyReceivingGrafanaAlerts]);

  const isDisabled = disabled || options.length === 1;
  const label = isDisabled ? 'Alertmanager' : 'Choose Alertmanager';

  // User may have selected an alertmanager elsewhere in the application that has then ended up being filtered out
  // due to "actions" requirements
  // In this case, we default back to Grafana AM
  const selectedValue = options.some((am) => am.value === selectedAlertmanager)
    ? selectedAlertmanager
    : GRAFANA_RULES_SOURCE_NAME;

  return (
    <InlineField className={styles.field} label={label} disabled={isDisabled} data-testid="alertmanager-picker">
      <Select
        aria-label={label}
        width={29}
        className="ds-picker select-container"
        backspaceRemovesValue={false}
        onChange={(value) => {
          if (value?.value) {
            setSelectedAlertmanager(value.value);
          }
        }}
        options={options}
        maxMenuHeight={500}
        noOptionsMessage="No datasources found"
        value={selectedValue}
        getOptionLabel={(o) => o.label}
      />
    </InlineField>
  );
};

const getStyles = (theme: GrafanaTheme2) => ({
  field: css({
    margin: 0,
  }),
});
