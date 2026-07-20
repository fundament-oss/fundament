#!/usr/bin/env node
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');

// Find all proto files in the organization API proto directory
const protoDir = path.join(__dirname, '..', 'organization-api', 'pkg', 'proto', 'v1');

function findProtoFiles(dir) {
  let results = [];
  const list = fs.readdirSync(dir);

  list.forEach((file) => {
    const filePath = path.join(dir, file);
    const stat = fs.statSync(filePath);

    if (stat && stat.isDirectory()) {
      results = results.concat(findProtoFiles(filePath));
    } else if (file.endsWith('.proto')) {
      results.push(filePath);
    }
  });

  return results;
}

const outputPath = path.join(__dirname, 'src', 'proto-version.gen.ts');

function write(version) {
  fs.writeFileSync(
    outputPath,
    `// Auto-generated file - do not edit
export default '${version}';
`,
  );
}

// Standalone checkouts (e.g. the Vercel demo deploy) only contain this directory,
// so the sibling organization-api is absent. The version is solely used for the
// live server/client handshake in app.config.ts, which the mock-backed demo never
// performs — so a placeholder is correct rather than a build failure.
if (!fs.existsSync(protoDir)) {
  write('unknown');
  console.log(`No proto directory at ${protoDir}; wrote placeholder proto version`);
  process.exit(0);
}

try {
  const protoFiles = findProtoFiles(protoDir);

  if (protoFiles.length === 0) {
    console.error('No proto files found');
    process.exit(1);
  }

  // Sort files for consistent ordering
  protoFiles.sort();

  // Create hash from all proto file contents
  const hash = crypto.createHash('sha256');

  protoFiles.forEach((file) => {
    const content = fs.readFileSync(file, 'utf8');
    hash.update(content);
  });

  // Get first 12 characters of the hash
  const version = hash.digest('hex').substring(0, 12);

  write(version);

  console.log(`Generated proto version: ${version}`);
} catch (error) {
  console.error('Error generating proto version:', error);
  process.exit(1);
}
