import fs from "node:fs";
import path from "node:path";
import type { NextConfig } from "next";

const parentRoot = path.join(__dirname, "../../../..");
const workspaceRoot = fs.existsSync(path.join(parentRoot, "servora"))
  ? parentRoot
  : path.join(__dirname, "../../..");

const nextConfig: NextConfig = {
  outputFileTracingRoot: workspaceRoot,
  transpilePackages: ["@plateau/api"],
  turbopack: {
    root: workspaceRoot,
  },
};

export default nextConfig;
