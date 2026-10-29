import { check } from "k6";
import http from "k6/http";

const BASE = __ENV.BASE_URL || "http://localhost:8080";

function randomFree() {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let result = "free_";

  for (let i = 0; i < 8; i++) {
    const randomIndex = Math.floor(Math.random() * chars.length);
    result += chars[randomIndex];
  }

  return result;
}

function randomTaken() {
  const n = Math.floor(Math.random() * 100000);
  return `user${n}`;
}

export const options = {
  vus: 50,
  duration: "30s",
};

export default function () {
  const name = Math.random() < 0.9 ? randomFree() : randomTaken();
  const res = http.get(`${BASE}/api/check?u=${name}`, {
    tags: { name: "check" },
  });

  check(res, {
    "status is 200": (r) => r.status === 200,
  });
}
