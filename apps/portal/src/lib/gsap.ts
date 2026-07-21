"use client";

import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { ScrollToPlugin } from "gsap/ScrollToPlugin";
import { Observer } from "gsap/Observer";
import { useGSAP } from "@gsap/react";

if (typeof window !== "undefined") {
  gsap.registerPlugin(ScrollTrigger, ScrollToPlugin, Observer, useGSAP);
}

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";
const FINE_MOTION = "(prefers-reduced-motion: no-preference)";

export { gsap, ScrollTrigger, useGSAP, REDUCED_MOTION, FINE_MOTION };
