import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { type Art, type ArtLayer } from '@core'

export const useArtsStore = defineStore('arts', () => {
    // // art // //

    const art = ref<Art>({
        name: 'new art',
        layers: [{ name: 'layer 0', dataURL: '' }]
    })

    function applyArt(newArt: Art) {
        art.value = newArt
        currentLayerIndex.value = newArt.layers.length - 1
    }

    // // layers // //

    const currentLayerIndex = ref(0)
    const deletedLayers = ref<ArtLayer[]>([])

    const currentLayer = computed<ArtLayer>(() => art.value.layers[currentLayerIndex.value])

    function addLayer(layer: ArtLayer) {
        art.value.layers.push(layer)
    }

    function createLayer() {
        const newLayerIndex = art.value.layers.length
        art.value.layers.push({ name: `layer ${newLayerIndex}`, dataURL: '' })
    }

    function deleteLayer(layerIndex: number) {
        deletedLayers.value.push(art.value.layers[layerIndex])
        art.value.layers = art.value.layers.filter((layer, i) => i !== layerIndex)
    }

    return { art, applyArt, currentLayerIndex, deletedLayers, currentLayer, addLayer, createLayer, deleteLayer }
})
