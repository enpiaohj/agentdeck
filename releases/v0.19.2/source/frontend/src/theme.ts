import { webDarkTheme, webLightTheme, type Theme } from '@fluentui/react-components'

const typography = {
  fontFamilyBase: '"Segoe UI Variable", "Segoe UI", "Microsoft YaHei", sans-serif',
  fontSizeBase200: '12px', fontSizeBase300: '13px', fontSizeBase400: '14px',
  lineHeightBase200: '18px', lineHeightBase300: '20px', lineHeightBase400: '22px',
  borderRadiusSmall: '4px', borderRadiusMedium: '6px', borderRadiusLarge: '8px', borderRadiusXLarge: '10px',
}

// Neutral desktop surfaces; blue is reserved for focus, selection and primary actions.
export const lightTheme: Theme = {
  ...webLightTheme, ...typography,
  colorBrandBackground: '#355fd0', colorBrandBackgroundHover: '#2c52bd', colorBrandBackgroundPressed: '#24469e',
  colorBrandBackground2: '#e9eefb', colorBrandBackground2Hover: '#e0e8fa',
  colorBrandForeground1: '#3158bd', colorBrandForeground2: '#3158bd', colorBrandStroke1: '#355fd0',
  colorNeutralForeground1: '#222937', colorNeutralForeground2: '#505969', colorNeutralForeground3: '#677080', colorNeutralForeground4: '#677080',
  colorNeutralBackground1: '#ffffff', colorNeutralBackground2: '#f5f6f8', colorNeutralBackground3: '#edeff3', colorNeutralBackground4: '#e5e8ee',
  colorNeutralBackground1Hover: '#f3f5f8', colorNeutralBackground3Hover: '#e8ebf1',
  colorNeutralStroke1: '#cbd0d9', colorNeutralStroke2: '#e3e6eb', colorNeutralStroke3: '#edf0f3',
  shadow4: '0 2px 8px #1018280d', shadow8: '0 4px 16px #10182814', shadow16: '0 8px 28px #1018281a',
}

export const darkTheme: Theme = {
  ...webDarkTheme, ...typography,
  colorBrandBackground: '#3b76ef', colorBrandBackgroundHover: '#4a86fa', colorBrandBackgroundPressed: '#2964dd',
  colorBrandBackground2: '#29354e', colorBrandBackground2Hover: '#32405b',
  colorBrandForeground1: '#a4bcff', colorBrandForeground2: '#a4bcff', colorBrandStroke1: '#7a9bf1',
  colorNeutralForeground1: '#ebedf2', colorNeutralForeground2: '#b9c0cd', colorNeutralForeground3: '#a0a8b7', colorNeutralForeground4: '#969faf',
  colorNeutralBackground1: '#1b1e25', colorNeutralBackground2: '#16191f', colorNeutralBackground3: '#282d37', colorNeutralBackground4: '#303745',
  colorNeutralBackground1Hover: '#242932', colorNeutralBackground3Hover: '#333b49',
  colorNeutralStroke1: '#4c5668', colorNeutralStroke2: '#343943', colorNeutralStroke3: '#2c313b',
  shadow4: '0 2px 8px #00000020', shadow8: '0 4px 16px #00000030', shadow16: '0 8px 28px #00000040',
}
