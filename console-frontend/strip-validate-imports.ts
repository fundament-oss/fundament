import { readdir, readFile, writeFile, rm } from "fs/promises";
import { join } from "path";

const generatedDir = join(import.meta.dir, "src", "generated");
const v1Dir = join(generatedDir, "v1");
const bufDir = join(generatedDir, "buf");

const files = await readdir(v1Dir);

await Promise.all(
  files
    .filter((f) => f.endsWith("_pb.ts"))
    .map(async (file) => {
      const filePath = join(v1Dir, file);
      let content = await readFile(filePath, "utf-8");

      if (!content.includes("file_buf_validate_validate")) return;

      // Remove the import line
      content = content.replace(
        /import \{ file_buf_validate_validate \} from "\.\.\/buf\/validate\/validate_pb";\n/,
        "",
      );

      // Remove from fileDesc dependency arrays
      content = content.replace(/file_buf_validate_validate,\s*/g, "");
      content = content.replace(/,\s*file_buf_validate_validate/g, "");

      await writeFile(filePath, content);
      console.log(`Stripped validate import from ${file}`);
    }),
);

await rm(bufDir, { recursive: true, force: true });
console.log("Deleted buf/ directory");
