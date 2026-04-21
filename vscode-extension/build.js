// @ts-check
const esbuild = require('esbuild');

/** esbuild plugin: redirect UMD imports to ESM equivalents */
const umdToEsmPlugin = {
  name: 'umd-to-esm',
  setup(build) {
    const path = require('path');
    const filter = /vscode-(json-languageservice|languageserver-types)[/\\]lib[/\\]umd/;
    build.onResolve({ filter }, (args) => {
      const esm = args.path.replace(/lib[/\\]umd/, 'lib/esm');
      const resolved = require.resolve(esm, { paths: [args.resolveDir] });
      return { path: resolved };
    });

    // Stub @vscode/l10n — just return the input string (English-only, no file loading)
    build.onResolve({ filter: /^@vscode\/l10n$/ }, () => ({
      path: '@vscode/l10n',
      namespace: 'l10n-stub',
    }));
    build.onLoad({ filter: /.*/, namespace: 'l10n-stub' }, () => ({
      contents: `
        module.exports = {
          t: (msg) => typeof msg === 'string' ? msg : msg.message || '',
          config: () => {},
          l10nBundle: undefined,
        };
      `,
      loader: 'js',
    }));
  },
};

async function build() {
  // Extension bundle
  await esbuild.build({
    entryPoints: ['src/extension.ts'],
    bundle: true,
    outfile: 'out/extension.js',
    external: ['vscode'],
    format: 'cjs',
    platform: 'node',
    sourcemap: true,
  });

  // YAML language server bundle
  await esbuild.build({
    entryPoints: [require.resolve('yaml-language-server/out/server/src/server.js')],
    bundle: true,
    outfile: 'out/yaml-server.js',
    format: 'cjs',
    platform: 'node',
    mainFields: ['module', 'main'],
    minify: true,
    plugins: [umdToEsmPlugin],
  });

  // Copy @types/node into the output so our TS language service can resolve
  // Node.js built-in modules regardless of the user's project setup.
  const fs = require('fs');
  const path = require('path');
  const srcTypes = path.join(__dirname, 'node_modules', '@types', 'node');
  const destTypes = path.join(__dirname, 'out', 'types', '@types', 'node');

  function copyDirRecursive(src, dest) {
    fs.mkdirSync(dest, { recursive: true });
    for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
      const srcPath = path.join(src, entry.name);
      const destPath = path.join(dest, entry.name);
      if (entry.isDirectory()) {
        copyDirRecursive(srcPath, destPath);
      } else if (entry.name.endsWith('.d.ts') || entry.name === 'package.json') {
        fs.copyFileSync(srcPath, destPath);
      }
    }
  }
  copyDirRecursive(srcTypes, destTypes);
}

build().catch((e) => {
  console.error(e);
  process.exit(1);
});
