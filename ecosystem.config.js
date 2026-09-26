module.exports = {
  apps: [
    {
      name: "backend-monet",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/finance/backend",
      script: "./racks-backend",
      autorestart: true,
      watch: false
    },
    {
      name: "frontend-monet",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/finance/frontend",
      script: "npm",
      args: "run dev",
      autorestart: true,
      watch: false
    },
    {
      name: "scraping-api",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/python",
      script: "apiscrap.py",
      interpreter: "python3",
      args: "--host=0.0.0.0 --port=5000",
      autorestart: true,
      watch: false
    },
    {
      name: "streamhub-app",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/streamhub",
      script: "artisan",
      interpreter: "php",
      args: "serve --host=0.0.0.0 --port=8001",
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: "1G"
    },
    {
      name: "stream-video-download-worker",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/streamhub",
      script: "artisan",
      interpreter: "php",
      args: "queue:work database --queue=video-downloads --sleep=1 --tries=3 --timeout=21600",
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: "1G",
      kill_timeout: 21650000
    },
    {
      name: "stream-fast-start-worker",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/streamhub",
      script: "artisan",
      interpreter: "php",
      args: "queue:work database --queue=video-processing --sleep=1 --tries=1 --timeout=7500",
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: "500M",
      kill_timeout: 7550000
    },
    {
      name: "scenescrap",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/python",
      script: "apiscrap.py",
      interpreter: "python3",
      autorestart: false,
      watch: false
    },
    {
      name: "full-scrap",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/python",
      script: "titlescene.py",
      interpreter: "python3",
      autorestart: false,
      watch: false
    },
    {
      name: "english-test",
      cwd: "/media/devmon/sda1-usb-JMicron_Tech_DD5/Rio/english-test",
      script: "./dist/english-practice-arm64",
      instances: 1,
      exec_mode: "fork",
      autorestart: true,
      watch: false,
      max_memory_restart: "500M",
      env: {
        APP_ADDR: ":8080"
      }
    }
  ]
};
