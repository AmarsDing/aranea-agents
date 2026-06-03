module.exports = {
  '*.go': [
    'gofmt -w',
  ],
  'web/src/**/*.{ts,vue}': [
    (filenames) => {
      const relativeFiles = filenames.map(f => f.replace(/^web[\\/]/, ''));
      if (relativeFiles.length === 0) return [];
      return [
        `cd web && npx stylelint --fix ${relativeFiles.map(f => `"${f}"`).join(' ')}`,
      ];
    },
  ],
  '*.proto': ['buf format -w'],
};
