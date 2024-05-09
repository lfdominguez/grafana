import { css, cx } from '@emotion/css';
import React, { PropsWithChildren, useEffect, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useCopyToClipboard } from 'react-use';

import { GrafanaTheme2, SelectableValue } from '@grafana/data';
import {
  Button,
  Drawer,
  IconButton,
  Input,
  RadioButtonGroup,
  Select,
  Stack,
  Text,
  TextArea,
  useStyles2,
} from '@grafana/ui';
import { useAlertmanagerConfig } from 'app/features/alerting/unified/hooks/useAlertmanagerConfig';
import { useAlertmanager } from 'app/features/alerting/unified/state/AlertmanagerContext';
import { NotificationChannelOption } from 'app/types';

import { defaultPayloadString } from '../../TemplateForm';

import { TemplateContentAndPreview } from './TemplateContentAndPreview';

interface TemplatesPickerProps {
  onSelect: (temnplate: string) => void;
  option: NotificationChannelOption;
  valueInForm: string;
}
export function TemplatesPicker({ onSelect, option, valueInForm }: TemplatesPickerProps) {
  const [showTemplates, setShowTemplates] = React.useState(false);

  return (
    <>
      <Button
        icon="edit"
        tooltip={'Edit using existing templates.'}
        onClick={() => setShowTemplates(true)}
        variant="secondary"
        size="sm"
        aria-label={'Select available template from the list of available templates.'}
      >
        {`Edit ${option.label}`}
      </Button>

      {showTemplates && (
        <Drawer title={`Edit ${option.label}`} size="md" onClose={() => setShowTemplates(false)}>
          <TemplateSelector
            onSelect={onSelect}
            onClose={() => setShowTemplates(false)}
            option={option}
            valueInForm={valueInForm}
          />
        </Drawer>
      )}
    </>
  );
}

type TemplateFieldOption = 'Existing' | 'Custom';

function parseTemplates(templatesString: string): Template[] {
  const result: Template[] = [];
  const regex = /{{ define "(.*?)" }}\n(.*?)\n{{ end }}/gs;

  let match;
  while ((match = regex.exec(templatesString)) !== null) {
    result.push({
      name: match[1],
      content: match[2],
    });
  }

  return result;
}

export interface Template {
  name: string;
  content: string;
}
interface TemplateSelectorProps {
  onSelect: (template: string) => void;
  onClose: () => void;
  option: NotificationChannelOption;
  valueInForm: string;
}
function TemplateSelector({ onSelect, onClose, option, valueInForm }: TemplateSelectorProps) {
  const styles = useStyles2(getStyles);
  const [template, setTemplate] = React.useState<Template | undefined>(undefined);
  const [inputToUpdate, setInputToUpdate] = React.useState<string>('');
  const [inputToUpdateCustom, setInputToUpdateCustom] = React.useState<string>(valueInForm);

  const { selectedAlertmanager } = useAlertmanager();
  const { data, error } = useAlertmanagerConfig(selectedAlertmanager);
  const [templateOption, setTemplateOption] = React.useState<TemplateFieldOption>('Existing');
  const [_, copyToClipboard] = useCopyToClipboard();

  const templateOptions: Array<SelectableValue<TemplateFieldOption>> = [
    { label: 'Selecting existing template', value: 'Existing' },
    { label: `Enter custom ${option.label}`, value: 'Custom' },
  ];

  useEffect(() => {
    if (template) {
      setInputToUpdate(getUseTemplateText(template.name));
    }
  }, [template]);

  function onCustomTemplateChange(customInput: string) {
    setInputToUpdateCustom(customInput);
  }

  const onTemplateOptionChange = (option: TemplateFieldOption) => {
    setTemplateOption(option);
  };
  const options = useMemo(
    () =>
      Object.entries(data?.template_files ?? {})
        .map(([name, content]) => {
          const templates: Template[] = parseTemplates(content);
          return templates.map((template) => ({
            label: template.name,
            value: {
              name: template.name,
              content: template.content,
            },
          }));
        })
        .flat(),
    [data]
  );

  if (error) {
    return <div>Error loading templates</div>;
  }

  if (!data) {
    return <div>Loading...</div>;
  }

  return (
    <Stack direction="column" gap={1} justifyContent="space-between" height="100%">
      <Stack direction="column" gap={1}>
        <RadioButtonGroup
          options={templateOptions}
          value={templateOption}
          onChange={onTemplateOptionChange}
          className={styles.templateTabOption}
        />

        {templateOption === 'Existing' ? (
          <Stack direction="column" gap={1}>
            <Stack direction="row" gap={1} alignItems="center">
              <Select<Template>
                aria-label="Template"
                onChange={(value: SelectableValue<Template>, _) => {
                  setTemplate(value?.value);
                }}
                options={options}
                width={50}
              />
              <IconButton
                tooltip="Copy selected template to clipboard. You can use it in the custom tab."
                onClick={() => copyToClipboard(getUseTemplateText(template?.name ?? ''))}
                name="copy"
              />
            </Stack>

            <TemplateContentAndPreview
              templateContent={template?.content ?? ''}
              payload={defaultPayloadString}
              templateName={template?.name ?? ''}
              setPayloadFormatError={() => {}}
              className={cx(styles.templatePreview, styles.minEditorSize)}
              payloadFormatError={null}
            />
          </Stack>
        ) : (
          <OptionCustomfield
            option={option}
            onCustomTemplateChange={onCustomTemplateChange}
            initialValue={inputToUpdateCustom}
          />
        )}
      </Stack>
      <div className={styles.actions}>
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="primary"
          onClick={() => {
            onSelect(templateOption === 'Custom' ? inputToUpdateCustom : inputToUpdate);
            onClose();
          }}
        >
          Save
        </Button>
      </div>
    </Stack>
  );
}

function OptionCustomfield({
  option,
  onCustomTemplateChange,
  initialValue,
}: {
  option: NotificationChannelOption;
  onCustomTemplateChange(customInput: string): void;
  initialValue: string;
}) {
  switch (option.element) {
    case 'textarea':
      return (
        <Stack direction="row" gap={1} alignItems="center">
          <TextArea
            placeholder={option.placeholder}
            onChange={(e) => onCustomTemplateChange(e.currentTarget.value)}
            defaultValue={initialValue}
          />
        </Stack>
      );
    case 'input':
      return (
        <Stack direction="row" gap={1} alignItems="center">
          <Input
            type={option.inputType}
            placeholder={option.placeholder}
            onChange={(e) => onCustomTemplateChange(e.currentTarget.value)}
            defaultValue={initialValue}
          />
        </Stack>
      );
    default:
      return null;
  }
}

function getUseTemplateText(templateName: string) {
  return `{{ template "${templateName}" . }}`;
}

function getTemplateName(useTemplateText: string): string | null {
  const match = useTemplateText.match(/\{\{\s*template\s*"(.*)"\s*\.\s*\}\}/);
  return match ? match[1] : null;
}
function matchesOnlyOneTemplate(templateContent: string) {
  const pattern = /\{\{\s*template\s*".*?"\s*\.\s*\}\}/g;
  const matches = templateContent.match(pattern);
  return matches && matches.length === 1;
}

interface WrapWithTemplateSelectionProps extends PropsWithChildren {
  useTemplates: boolean;
  onSelectTemplate: (template: string) => void;
  option: NotificationChannelOption;
  name: string;
}
export function WrapWithTemplateSelection({
  useTemplates,
  onSelectTemplate,
  option,
  name,
  children,
}: WrapWithTemplateSelectionProps) {
  const { getValues } = useFormContext();
  const value: string = getValues(name) ?? '';
  const emptyValue = value === '' || value === undefined;
  const onlyOneTemplate = value ? matchesOnlyOneTemplate(value) : false;
  const styles = useStyles2(getStyles);
  if (emptyValue) {
    return (
      <div className={styles.inputContainer}>
        <Stack direction="row" gap={1} alignItems="center">
          {useTemplates && (
            <TemplatesPicker onSelect={onSelectTemplate} option={option} valueInForm={getValues(name) ?? ''} />
          )}
        </Stack>
      </div>
    );
  }
  if (onlyOneTemplate) {
    return (
      <div className={styles.inputContainer}>
        <Stack direction="row" gap={1} alignItems="center">
          <Text variant="bodySmall">{`Template: ${getTemplateName(value)}`}</Text>
          {useTemplates && (
            <TemplatesPicker onSelect={onSelectTemplate} option={option} valueInForm={getValues(name) ?? ''} />
          )}
        </Stack>
      </div>
    );
  }
  // custom template  field
  return (
    <div className={styles.inputContainer}>
      <Stack direction="row" gap={1} alignItems="center">
        {children}
        {useTemplates && (
          <TemplatesPicker onSelect={onSelectTemplate} option={option} valueInForm={getValues(name) ?? ''} />
        )}
      </Stack>
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  actions: css({
    flex: 0,
    justifyContent: 'flex-end',
    display: 'flex',
    gap: theme.spacing(1),
  }),
  templatePreview: css({
    flex: 1,
    display: 'flex',
  }),
  templateTabOption: css({
    width: 'fit-content',
  }),
  minEditorSize: css({
    minHeight: 300,
    minWidth: 300,
  }),
  inputContainer: css({
    marginTop: theme.spacing(1.5),
  }),
});
