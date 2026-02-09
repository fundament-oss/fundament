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

  // Write version to a TypeScript file
  const outputPath = path.join(__dirname, 'src', 'proto-version.ts');
  const content = `// Auto-generated file - do not edit
export default '${version}';
`;

  fs.writeFileSync(outputPath, content);

  console.log(`Generated proto version: ${version}`);
} catch (error) {
  console.error('Error generating proto version:', error);
  process.exit(1);
}
