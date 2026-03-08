import { useState } from 'react'
import MapView, { type BBox } from './components/MapView'
import SearchView, { type Restaurant, type ChunkInfo } from './components/SearchView'
import './App.css'

function App() {
  const [bbox, setBbox] = useState<BBox | null>(null)
  const [restaurants, setRestaurants] = useState<Restaurant[]>([])
  const [chunks, setChunks] = useState<ChunkInfo[]>([])

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[#f8f8f7]">
      {/* Left — map */}
      <div className="flex-1 relative">
        <MapView
          bbox={bbox}
          onBboxChange={setBbox}
          restaurants={restaurants}
          chunks={chunks}
        />
      </div>

      {/* Divider */}
      <div className="w-px bg-gray-200 shrink-0" />

      {/* Right — search */}
      <div className="w-[420px] shrink-0 flex flex-col">
        <SearchView
          bbox={bbox}
          onResults={setRestaurants}
          onChunksChange={setChunks}
        />
      </div>
    </div>
  )
}

export default App
