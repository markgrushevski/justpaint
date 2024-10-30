import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { type Art, type ArtLayer } from '@core'

export const useArtsStore = defineStore('arts', () => {
    const deletedLayers = ref<ArtLayer[]>([])

    const art = ref<Art>({
        name: 'new art',
        layers: [{ name: 'layer 0', dataURL: '' }]
    })

    const currentLayerIndex = ref(0)

    const currentLayer = computed<ArtLayer>(() => {
        return art.value.layers[currentLayerIndex.value]
    })

    function addLayer(layer: ArtLayer) {
        art.value.layers.push(layer)
    }

    function createLayer() {
        const newLayerIndex = art.value.layers.length

        art.value.layers.push({ name: `layer ${newLayerIndex}`, dataURL: '' })
    }

    function removeLayer(layerIndex: number) {
        deletedLayers.value.push(art.value.layers[layerIndex])
        art.value.layers = art.value.layers.filter((layer, i) => i !== layerIndex)
    }

    return { art, currentLayerIndex, currentLayer }
})
