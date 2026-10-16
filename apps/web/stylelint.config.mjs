// Stylelint 门禁配置（EP-FE-03 颜色 token 化 + 风格规范）
//
// 边界说明：.sass 缩进语法在 stylelint 17 生态中无可用 customSyntax——
// postcss-sass@0.5.0 已停止维护（AST 缺 node.source，规则命中即崩），
// sugarss 与 Sass 缩进语法不兼容（parse 即崩）。
// 因此 **/*.sass 文件与 <style lang="sass"> 块在此忽略，
// 其颜色门禁由 scripts/check-color-tokens.mjs（pnpm color:gate）承担。
// <style lang="sass"> 块在 postcss-html 下因无对应 syntax 被自动跳过，不会报错。

export default {
  extends: ["stylelint-config-standard", "stylelint-config-recommended-vue"],
  rules: {
    "color-no-hex": [
      true,
      {
        message:
          "Use CSS custom properties (var(--*)) instead of hardcoded hex/rgb colors (EP-FE-03)",
      },
    ],
    "color-named": [
      "never",
      { message: "Use CSS custom properties instead of named colors" },
    ],
    "selector-class-pattern": [
      "^([a-z][a-z0-9]*)((-[a-z0-9]+)|(__[a-z][a-z0-9]*)|--[a-z][a-z0-9]*)*$|^(q-)",
      {
        resolveNestedSelectors: true,
        message:
          "Expected class selector to be kebab-case or BEM (block__element--modifier); Quasar q- prefixed classes are allowed",
      },
    ],
    "property-no-vendor-prefix": [
      true,
      {
        // Safari 兼容白名单：毛玻璃 / 渐变文字裁剪 / 文本填充 / 选择抑制
        ignoreProperties: [
          "backdrop-filter",
          "-webkit-backdrop-filter",
          "background-clip",
          "-webkit-background-clip",
          "text-fill-color",
          "-webkit-text-fill-color",
          "user-select",
          "-webkit-user-select",
        ],
        message:
          "Vendor prefix not allowed except whitelisted Safari compat properties",
      },
    ],
    "value-no-vendor-prefix": [
      true,
      { ignoreValues: ["-webkit-backdrop-filter"] },
    ],
    "alpha-value-notation": "percentage",
    "color-function-notation": "modern",
    "declaration-property-value-keyword-no-deprecated": true,
  },
  overrides: [
    {
      files: ["**/*.scss"],
      customSyntax: "postcss-scss",
    },
    {
      // 缩进 Sass 无可用 customSyntax（见文件头说明），交 check-color-tokens.mjs 门禁
      files: ["**/*.sass"],
      rules: {},
    },
  ],
  ignoreFiles: ["dist/**", ".quasar/**", "node_modules/**", "**/*.sass"],
};
