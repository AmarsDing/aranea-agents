const path = require('path');

module.exports = {
  '*.go': [
    'gofmt -w',
    (filenames) => {
      // go vet requires all files in one directory; group by dir and vet each
      const dirs = [...new Set(filenames.map(f => path.dirname(f)))];
      return dirs.map(d => `go vet "${d}"`);
    },
  ],
  'web/src/**/*.{ts,vue}': [
    (filenames) => {
      // Strip 'web/' prefix since we cd into web directory
      const relativeFiles = filenames.map(f => f.replace(/^web[\\/]/, ''));
      if (relativeFiles.length === 0) return [];
      return [
        `cd web && npx eslint --fix --max-warnings=0 ${relativeFiles.map(f => `"${f}"`).join(' ')}`,
      ];
    },
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
