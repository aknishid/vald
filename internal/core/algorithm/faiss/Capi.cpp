#include <stdio.h>
#include <stdlib.h>
#include <faiss/IndexFlat.h>
#include <faiss/IndexIVFPQ.h>
#include <faiss/impl/AuxIndexStructures.h>
#include <faiss/index_io.h>
#include "Capi.h"

FaissStruct* faiss_create_index(
    const int d,
    const int nlist,
    const int m,
    const int nbits_per_idx) {
  faiss::IndexFlatL2 *quantizer = new faiss::IndexFlatL2(d);
  faiss::IndexIVFPQ *index = new faiss::IndexIVFPQ(quantizer, d, nlist, m, nbits_per_idx);
  FaissStruct *st = new FaissStruct{
    static_cast<FaissQuantizer>(quantizer),
    static_cast<FaissIndex>(index)
  };

  printf(__FUNCTION__);
  fflush(stdout);
  return st;
}

FaissStruct* faiss_read_index(const char* fname) {
  FaissStruct *st = new FaissStruct{
    static_cast<FaissQuantizer>(NULL),
    static_cast<FaissIndex>(faiss::read_index(fname))
  };

  printf(__FUNCTION__);
  fflush(stdout);
  return st;
}

void faiss_write_index(
    const FaissStruct* st,
    const char* fname) {
  faiss::write_index(static_cast<faiss::IndexIVFPQ*>(st->faiss_index), fname);

  printf(__FUNCTION__);
  fflush(stdout);
  return;
}

void faiss_train(
    const FaissStruct* st,
    const int nb,
    const float* xb) {
  printf("is_trained: %d\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->is_trained);
  printf("ntotal: %ld\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->ntotal);
  (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->train(nb, xb);
  printf("is_trained: %d\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->is_trained);
  printf("ntotal: %ld\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->ntotal);

  printf(__FUNCTION__);
  fflush(stdout);
  return;
}

void faiss_add(
    const FaissStruct* st,
    const int nb,
    const float* xb,
    const long* xids ) {
  printf("is_trained: %d\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->is_trained);
  printf("ntotal: %ld\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->ntotal);
  (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->add_with_ids(nb, xb, xids);
  printf("is_trained: %d\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->is_trained);
  printf("ntotal: %ld\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->ntotal);

  printf(__FUNCTION__);
  fflush(stdout);
  return;
}

void faiss_search(
    const FaissStruct* st,
    const int k,
    const int nq,
    const float* xq,
    long* I,
    float* D) {
  printf("is_trained: %d\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->is_trained);
  printf("ntotal: %ld\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->ntotal);
  (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->search(nq, xq, k, D, I);

  printf("I=\n");
  for(int i = 0; i < nq; i++) {
      for(int j = 0; j < k; j++) {
          printf("%5ld ", I[i * k + j]);
      }
      printf("\n");
  }
  printf("D=\n");
  for(int i = 0; i < nq; i++) {
      for(int j = 0; j < k; j++) {
          printf("%7g ", D[i * k + j]);
      }
      printf("\n");
  }

  printf(__FUNCTION__);
  fflush(stdout);
  return;
}

void faiss_remove(
    const FaissStruct* st,
    const int size,
    const long int* ids) {
  printf("is_trained: %d\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->is_trained);
  printf("ntotal: %ld\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->ntotal);
  faiss::IDSelectorArray sel(size, ids);
  (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->remove_ids(sel);
  printf("is_trained: %d\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->is_trained);
  printf("ntotal: %ld\n", (static_cast<faiss::IndexIVFPQ*>(st->faiss_index))->ntotal);

  printf(__FUNCTION__);
  fflush(stdout);
  return;
}

void faiss_free(FaissStruct* st) {
  free(st);

  printf(__FUNCTION__);
  fflush(stdout);
  return;
}
