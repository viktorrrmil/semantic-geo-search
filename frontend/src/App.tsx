import { useState } from 'react'
import MapView from './components/MapView'
import DataPanel from './components/DataPanel'
import S3Explorer from './components/S3Explorer'
import type { SpatialFeature } from './components/DataPanel'
import type { SelectedFile } from './components/S3Explorer'
import './App.css'

function App() {
  const [selectedFile, setSelectedFile] = useState<SelectedFile | null>(null)
  const [features, setFeatures] = useState<SpatialFeature[]>([])
  const [s3Open, setS3Open] = useState(false)

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[#f8f8f7]">
      {/* Left — map */}
      <div className="flex-1 relative">
        <MapView features={features} />

        {/* S3 Browser toggle — bottom-left of map, subtle */}
        <button
          onClick={() => setS3Open(v => !v)}
          title="S3 Browser"
          className={`absolute bottom-4 left-4 z-40 flex items-center gap-1.5 px-2.5 py-1.5 rounded-md border shadow-sm text-[11px] font-medium transition-colors ${
            s3Open
              ? 'bg-gray-800 text-white border-gray-700'
              : 'bg-white text-gray-500 border-gray-200 hover:border-gray-400 hover:text-gray-700'
          }`}
        >
          <svg width="13" height="13" viewBox="0 0 13 13" fill="none">
            <ellipse cx="6.5" cy="3.5" rx="4.5" ry="1.5" stroke="currentColor" strokeWidth="1" />
            <path d="M2 3.5v6c0 .83 2.01 1.5 4.5 1.5s4.5-.67 4.5-1.5v-6" stroke="currentColor" strokeWidth="1" />
            <path d="M2 6.5c0 .83 2.01 1.5 4.5 1.5s4.5-.67 4.5-1.5" stroke="currentColor" strokeWidth="1" />
          </svg>
          S3
        </button>

        <S3Explorer
          isOpen={s3Open}
          onClose={() => setS3Open(false)}
          selectedFilePath={selectedFile?.path ?? null}
          onFileSelect={file => {
            setSelectedFile(file)
            if (!file) {
              setFeatures([])
            }
          }}
        />
      </div>

      {/* Divider */}
      <div className="w-px bg-gray-200 shrink-0" />

      {/* Right — data panel */}
      <div className="w-[420px] shrink-0 flex flex-col">
        <DataPanel
          selectedFile={selectedFile}
          onOpenS3Browser={() => setS3Open(true)}
          onFeaturesChange={setFeatures}
        />
      </div>
    </div>
  )
}

export default App
