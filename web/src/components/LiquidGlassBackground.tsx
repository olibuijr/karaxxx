import { useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import * as THREE from 'three'

type LiquidGlassBackgroundProps = {
  className?: string
}

type ShaderUniforms = {
  uTime: THREE.IUniform<number>
  uResolution: THREE.IUniform<THREE.Vector2>
  uMouse: THREE.IUniform<THREE.Vector2>
}

const containerStyle: CSSProperties = {
  position: 'absolute',
  inset: 0,
  pointerEvents: 'none',
  zIndex: 0,
  overflow: 'hidden',
}

const canvasStyle: CSSProperties = {
  display: 'block',
  width: '100%',
  height: '100%',
}

const fallbackStyle: CSSProperties = {
  position: 'absolute',
  inset: 0,
  background: [
    'radial-gradient(circle at 30% 32%, rgba(229, 9, 20, 0.12), transparent 0 32%)',
    'radial-gradient(circle at 72% 38%, rgba(249, 115, 22, 0.1), transparent 0 28%)',
    'radial-gradient(circle at 50% 62%, rgba(255, 255, 255, 0.035), transparent 0 24%)',
    'radial-gradient(circle at 50% 50%, #14141c 0%, #0d0d13 62%, #09090d 100%)',
  ].join(', '),
}

export default function LiquidGlassBackground({
  className,
}: LiquidGlassBackgroundProps) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const [webglFailed, setWebglFailed] = useState(false)

  useEffect(() => {
    const container = containerRef.current
    const canvas = canvasRef.current

    if (!container || !canvas) {
      return
    }

    const vertexShader = `
      varying vec2 vUv;

      void main() {
        vUv = position.xy * 0.5 + 0.5;
        gl_Position = vec4(position.xy, 0.0, 1.0);
      }
    `

    const fragmentShader = `
      precision highp float;

      uniform float uTime;
      uniform vec2 uResolution;
      uniform vec2 uMouse;

      varying vec2 vUv;

      vec2 hash2(vec2 p) {
        p = vec2(dot(p, vec2(127.1, 311.7)), dot(p, vec2(269.5, 183.3)));
        return fract(sin(p) * 43758.5453123);
      }

      float valueNoise(vec2 p) {
        vec2 i = floor(p);
        vec2 f = fract(p);

        float a = dot(hash2(i + vec2(0.0, 0.0)), f - vec2(0.0, 0.0));
        float b = dot(hash2(i + vec2(1.0, 0.0)), f - vec2(1.0, 0.0));
        float c = dot(hash2(i + vec2(0.0, 1.0)), f - vec2(0.0, 1.0));
        float d = dot(hash2(i + vec2(1.0, 1.0)), f - vec2(1.0, 1.0));

        vec2 u = f * f * (3.0 - 2.0 * f);
        return mix(mix(a, b, u.x), mix(c, d, u.x), u.y);
      }

      float fbm(vec2 p) {
        float value = 0.0;
        float amplitude = 0.5;
        vec2 shift = vec2(31.416, 17.903);

        for (int octave = 0; octave < 4; octave++) {
          value += amplitude * valueNoise(p);
          p = p * 2.02 + shift;
          amplitude *= 0.5;
        }

        return value;
      }

      void main() {
        vec2 centeredUv = vUv - 0.5;
        centeredUv.x *= uResolution.x / max(uResolution.y, 1.0);

        float t = uTime * 0.05;
        vec2 mouseOffset = (uMouse - 0.5) * vec2(0.12, -0.12);
        vec2 p = centeredUv * 1.05 + mouseOffset;

        // Draped satin: slow fbm warps the fold axis so crimson highlights
        // drift like silk under a spotlight
        vec2 warp = vec2(
          fbm(p * 1.2 + vec2(0.0, t)),
          fbm(p * 1.2 + vec2(3.7, t * 0.8))
        );
        float foldPhase = p.y * 2.4 + p.x * 0.8 + warp.y * 3.2 + warp.x * 1.8 + t * 0.5;
        float folds = sin(foldPhase * 2.6);
        float sheen = pow(0.5 + 0.5 * folds, 3.5);
        float foldShadow = pow(0.5 - 0.5 * folds, 2.0);
        float drift = clamp(fbm(p * 0.8 + warp * 0.6) + 0.5, 0.0, 1.0);

        vec3 base = vec3(0.066, 0.048, 0.078);
        vec3 wine = vec3(0.38, 0.045, 0.10);
        vec3 crimson = vec3(0.898, 0.035, 0.078);
        vec3 hotPink = vec3(1.0, 0.22, 0.42);
        vec3 amber = vec3(0.976, 0.451, 0.086);

        vec3 color = mix(base, wine, 0.10 + 0.32 * drift);
        color += crimson * sheen * 0.24;
        color += hotPink * sheen * sheen * 0.07;
        color *= 1.0 - foldShadow * 0.35;

        // Bokeh: drifting out-of-focus club lights in warm reds and pinks
        for (int i = 0; i < 7; i++) {
          float fi = float(i);
          vec2 seed = hash2(vec2(fi * 1.61, fi * 2.71));
          vec2 center = (seed - 0.5) * vec2(2.1, 1.25);
          center.x += sin(t * (0.35 + seed.y * 0.5) + fi * 1.7) * 0.38;
          center.y += cos(t * (0.28 + seed.x * 0.4) + fi * 2.3) * 0.24;
          float d = length(centeredUv - center);
          float radius = 0.05 + seed.x * 0.11;
          float orb = smoothstep(radius, radius * 0.2, d);
          float rim = smoothstep(radius, radius * 0.82, d) - smoothstep(radius * 0.82, radius * 0.55, d);
          vec3 orbColor = mix(crimson, mix(hotPink, amber, seed.x), seed.y);
          color += orbColor * orb * (0.085 + seed.y * 0.075);
          color += orbColor * rim * 0.05;
        }

        float vignette = smoothstep(0.95, 0.22, length(centeredUv * vec2(0.88, 1.0)));
        color *= 0.68 + vignette * 0.32;

        color = clamp(color, vec3(0.0), vec3(0.40));

        gl_FragColor = vec4(color, 1.0);
      }
    `

    const uniforms: ShaderUniforms = {
      uTime: { value: 9.25 },
      uResolution: { value: new THREE.Vector2(1, 1) },
      uMouse: { value: new THREE.Vector2(0.5, 0.5) },
    }

    const reducedMotionMedia = window.matchMedia('(prefers-reduced-motion: reduce)')
    const targetMouse = new THREE.Vector2(0.5, 0.5)
    const currentMouse = new THREE.Vector2(0.5, 0.5)
    // Plain timestamp clock (THREE.Clock is deprecated in r184)
    let clockStart = performance.now()
    const clock = {
      start: () => { clockStart = performance.now() },
      getElapsedTime: () => (performance.now() - clockStart) / 1000,
    }

    let renderer: THREE.WebGLRenderer | null = null
    let geometry: THREE.PlaneGeometry | null = null
    let material: THREE.ShaderMaterial | null = null
    let resizeObserver: ResizeObserver | null = null
    let frameId: number | null = null

    const scene = new THREE.Scene()
    const camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1)

    const stopLoop = () => {
      if (frameId !== null) {
        cancelAnimationFrame(frameId)
        frameId = null
      }
    }

    const renderFrame = (timeValue: number) => {
      if (!renderer || !material) {
        return
      }

      currentMouse.lerp(targetMouse, 0.05)
      uniforms.uTime.value = timeValue
      uniforms.uMouse.value.copy(currentMouse)
      material.uniforms.uTime.value = uniforms.uTime.value
      material.uniforms.uMouse.value.copy(uniforms.uMouse.value)
      renderer.render(scene, camera)
    }

    const animate = () => {
      frameId = null

      if (document.hidden) {
        return
      }

      renderFrame(clock.getElapsedTime())

      if (!reducedMotionMedia.matches) {
        frameId = window.requestAnimationFrame(animate)
      }
    }

    const startLoop = () => {
      if (frameId !== null || reducedMotionMedia.matches || document.hidden) {
        return
      }

      frameId = window.requestAnimationFrame(animate)
    }

    const applySize = () => {
      if (!renderer) {
        return
      }

      const width = Math.max(container.clientWidth, 1)
      const height = Math.max(container.clientHeight, 1)
      renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
      renderer.setSize(width, height, false)
      uniforms.uResolution.value.set(width, height)
      if (material) {
        material.uniforms.uResolution.value.copy(uniforms.uResolution.value)
      }
      if (reducedMotionMedia.matches || document.hidden) {
        renderFrame(uniforms.uTime.value)
      }
    }

    const handlePointerMove = (event: PointerEvent) => {
      const width = Math.max(window.innerWidth, 1)
      const height = Math.max(window.innerHeight, 1)
      targetMouse.set(event.clientX / width, event.clientY / height)
    }

    const handleVisibilityChange = () => {
      if (document.hidden) {
        stopLoop()
        return
      }

      if (reducedMotionMedia.matches) {
        renderFrame(9.25)
        return
      }

      startLoop()
    }

    const handleContextLost = (event: Event) => {
      event.preventDefault()
      stopLoop()
    }

    const handleContextRestored = () => {
      clock.start()
      if (reducedMotionMedia.matches) {
        renderFrame(9.25)
        return
      }
      if (!document.hidden) {
        startLoop()
      }
    }

    try {
      renderer = new THREE.WebGLRenderer({
        canvas,
        antialias: false,
        alpha: true,
        powerPreference: 'high-performance',
      })
      renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
      renderer.setClearColor(0x000000, 0)

      geometry = new THREE.PlaneGeometry(2, 2)
      material = new THREE.ShaderMaterial({
        uniforms,
        vertexShader,
        fragmentShader,
        depthWrite: false,
        depthTest: false,
        transparent: true,
      })

      const quad = new THREE.Mesh(geometry, material)
      scene.add(quad)
      applySize()
    } catch {
      geometry?.dispose()
      material?.dispose()
      renderer?.dispose()
      geometry = null
      material = null
      renderer = null
      setWebglFailed(true)
      return () => {
        stopLoop()
        resizeObserver?.disconnect()
      }
    }

    resizeObserver = new ResizeObserver(() => {
      applySize()
    })
    resizeObserver.observe(container)

    window.addEventListener('pointermove', handlePointerMove, { passive: true })
    document.addEventListener('visibilitychange', handleVisibilityChange)
    canvas.addEventListener('webglcontextlost', handleContextLost)
    canvas.addEventListener('webglcontextrestored', handleContextRestored)

    if (reducedMotionMedia.matches) {
      renderFrame(9.25)
    } else {
      clock.start()
      startLoop()
    }

    return () => {
      stopLoop()
      window.removeEventListener('pointermove', handlePointerMove)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      canvas.removeEventListener('webglcontextlost', handleContextLost)
      canvas.removeEventListener('webglcontextrestored', handleContextRestored)
      resizeObserver?.disconnect()
      geometry?.dispose()
      material?.dispose()
      renderer?.dispose()
    }
  }, [])

  return (
    <div
      ref={containerRef}
      aria-hidden="true"
      className={className}
      style={containerStyle}
    >
      {webglFailed ? (
        <div style={fallbackStyle} />
      ) : (
        <canvas ref={canvasRef} style={canvasStyle} />
      )}
    </div>
  )
}
