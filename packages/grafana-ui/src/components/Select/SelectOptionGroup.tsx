import React from 'react';
import { GroupProps } from 'react-select';

export const SelectOptionGroup = ({
  children,
  cx,
  getClassNames,
  getStyles,
  Heading,
  headingProps,
  label,
  selectProps,
  theme,
}: GroupProps) => {
  return (
    <>
      <Heading
        cx={cx}
        getClassNames={getClassNames}
        getStyles={getStyles}
        selectProps={selectProps}
        theme={theme}
        {...headingProps}
      >
        {label}
      </Heading>
      {children}
    </>
  );
};
