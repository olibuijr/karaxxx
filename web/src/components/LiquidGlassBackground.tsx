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

        float slowTime = uTime * 0.055;
        vec2 mouseOffset = (uMouse - 0.5) * vec2(0.16, -0.16);
        vec2 baseUv = centeredUv * 1.45 + mouseOffset;

        vec2 warpA = vec2(
          fbm(baseUv * 1.15 + vec2(0.0, slowTime)),
          fbm(baseUv * 1.15 + vec2(4.3, slowTime + 2.6))
        );

        vec2 warpB = vec2(
          fbm(baseUv * 2.0 + warpA * 1.4 + vec2(slowTime * 0.7, 1.7)),
          fbm(baseUv * 2.0 - warpA * 1.2 + vec2(-2.1, slowTime * 0.6))
        );

        vec2 flowUv = baseUv + (warpA - 0.5) * 0.55 + (warpB - 0.5) * 0.28;
        float field = fbm(flowUv * 2.25 + warpB * 0.7 - slowTime * 0.45);
        float ridges = 1.0 - abs(field * 2.0 - 1.0);
        float caustic = pow(clamp(ridges, 0.0, 1.0), 3.8);
        float secondary = pow(clamp(fbm(flowUv * 3.6 - warpA * 0.8 + slowTime * 0.35) * 0.5 + 0.5, 0.0, 1.0), 6.0);
        float sparkle = pow(clamp(1.0 - abs(fbm(flowUv * 7.2 + warpB * 1.2 + slowTime * 0.2)), 0.0, 1.0), 12.0);

        vec3 baseColor = vec3(0.05098, 0.05098, 0.0745);
        vec3 redGlow = vec3(0.898, 0.035, 0.078);
        vec3 orangeGlow = vec3(0.976, 0.451, 0.086);
        vec3 whiteSpec = vec3(1.0);

        vec3 color = baseColor;
        color += redGlow * caustic * 0.11;
        color += orangeGlow * secondary * 0.085;
        color += whiteSpec * sparkle * 0.028;

        float vignette = smoothstep(0.86, 0.18, length(centeredUv * vec2(0.92, 1.02)));
        color *= 0.74 + vignette * 0.26;

        color = clamp(color, vec3(0.0), vec3(0.23));

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
    const clock = new THREE.Clock()

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
