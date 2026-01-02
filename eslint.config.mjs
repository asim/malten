export default [
  {
    languageOptions: {
      ecmaVersion: 2015,
      sourceType: "script",
      globals: {
        window: "readonly",
        document: "readonly",
        console: "readonly",
        localStorage: "readonly",
        navigator: "readonly",
        WebSocket: "readonly",
        setTimeout: "readonly",
        setInterval: "readonly",
        clearTimeout: "readonly",
        Date: "readonly",
        JSON: "readonly",
        Math: "readonly",
        location: "readonly",
        $: "readonly"
      }
    },
    rules: {
      "no-undef": "error",
      "no-unused-vars": "warn",
      "no-unreachable": "error",
      "no-constant-condition": "warn",
      "no-dupe-keys": "error",
      "no-duplicate-case": "error",
      "no-empty": "warn",
      "no-extra-semi": "warn",
      "no-func-assign": "error",
      "no-invalid-regexp": "error",
      "use-isnan": "error",
      "valid-typeof": "error"
    }
  }
];
