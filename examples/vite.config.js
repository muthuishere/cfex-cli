import { defineConfig } from "vite";

export default defineConfig(({ command, mode }) => {
  const port = process.argv.includes("--port")
    ? parseInt(process.argv[process.argv.indexOf("--port") + 1])
    : 8080;

  return {
    server: {
      port,
      watch: {
        usePolling: true,
      },
    },
  };
});
